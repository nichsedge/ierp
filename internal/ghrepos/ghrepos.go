package ghrepos

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const graphQLQuery = `query($after:String){
  viewer {
    login
    repositories(
      first: 30,
      after: $after,
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER],
      orderBy: {field: UPDATED_AT, direction: DESC}
    ) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        name
        nameWithOwner
        url
        description
        homepageUrl
        isPrivate
        isFork
        isArchived
        isTemplate
        isDisabled
        createdAt
        updatedAt
        pushedAt
        primaryLanguage { name }
        defaultBranchRef {
          name
          target {
            ... on Commit { oid }
          }
        }
        stargazerCount
        forkCount
        watchers { totalCount }
        issues(states: OPEN) { totalCount }
        pullRequests(states: OPEN) { totalCount }
        repositoryTopics(first: 50) {
          nodes { topic { name } }
        }
        licenseInfo { spdxId name }
        owner {
          login
          url
        }
      }
    }
  }
}`

type graphQLResponse struct {
	Data struct {
		Viewer struct {
			Login        string `json:"login"`
			Repositories struct {
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []repoNode `json:"nodes"`
			} `json:"repositories"`
		} `json:"viewer"`
	} `json:"data"`
}

type repoNode struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	NameWithOwner   string  `json:"nameWithOwner"`
	URL             string  `json:"url"`
	Description     *string `json:"description"`
	HomepageURL     *string `json:"homepageUrl"`
	IsPrivate       bool    `json:"isPrivate"`
	IsFork          bool    `json:"isFork"`
	IsArchived      bool    `json:"isArchived"`
	IsTemplate      bool    `json:"isTemplate"`
	IsDisabled      bool    `json:"isDisabled"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	PushedAt        *string `json:"pushedAt"`
	PrimaryLanguage *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
	DefaultBranchRef *struct {
		Name   string `json:"name"`
		Target *struct {
			OID string `json:"oid"`
		} `json:"target"`
	} `json:"defaultBranchRef"`
	StargazerCount   int `json:"stargazerCount"`
	ForkCount        int `json:"forkCount"`
	Watchers         struct{ TotalCount int `json:"totalCount"` } `json:"watchers"`
	Issues           struct{ TotalCount int `json:"totalCount"` } `json:"issues"`
	PullRequests     struct{ TotalCount int `json:"totalCount"` } `json:"pullRequests"`
	RepositoryTopics struct {
		Nodes []struct {
			Topic struct {
				Name string `json:"name"`
			} `json:"topic"`
		} `json:"nodes"`
	} `json:"repositoryTopics"`
	LicenseInfo *struct {
		SpdxID *string `json:"spdxId"`
		Name   string  `json:"name"`
	} `json:"licenseInfo"`
	Owner struct {
		Login string `json:"login"`
		URL   string `json:"url"`
	} `json:"owner"`
}

// SyncGitHubRepos pulls repositories using `gh api graphql` and upserts them into events.db.
func SyncGitHubRepos(ctx context.Context, db *sql.DB) (int, error) {
	stmt, err := db.PrepareContext(ctx, `
		INSERT INTO github_repositories (
			repo_id, name, full_name, owner_login, owner_url, html_url,
			homepage, description, topics, language, private, fork,
			archived, template, disabled, created_at, updated_at, pushed_at,
			default_branch, default_branch_oid, stargazers_count, watchers_count,
			forks_count, open_issues_count, open_prs_count, license_spdx, license_name, synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'))
		ON CONFLICT(repo_id) DO UPDATE SET
			name=excluded.name, full_name=excluded.full_name, owner_login=excluded.owner_login,
			owner_url=excluded.owner_url, html_url=excluded.html_url, homepage=excluded.homepage,
			description=excluded.description, topics=excluded.topics, language=excluded.language,
			private=excluded.private, fork=excluded.fork, archived=excluded.archived,
			template=excluded.template, disabled=excluded.disabled, created_at=excluded.created_at,
			updated_at=excluded.updated_at, pushed_at=excluded.pushed_at, default_branch=excluded.default_branch,
			default_branch_oid=excluded.default_branch_oid, stargazers_count=excluded.stargazers_count,
			watchers_count=excluded.watchers_count, forks_count=excluded.forks_count,
			open_issues_count=excluded.open_issues_count, open_prs_count=excluded.open_prs_count,
			license_spdx=excluded.license_spdx, license_name=excluded.license_name,
			synced_at=excluded.synced_at;
	`)
	if err != nil {
		return 0, fmt.Errorf("failed preparing upsert statement: %w", err)
	}
	defer stmt.Close()

	var afterCursor *string
	totalSynced := 0

	for {
		args := []string{"api", "graphql", "-f", fmt.Sprintf("query=%s", graphQLQuery)}
		if afterCursor != nil {
			args = append(args, "-F", fmt.Sprintf("after=%s", *afterCursor))
		} else {
			args = append(args, "-F", "after=null")
		}

		cmd := exec.CommandContext(ctx, "gh", args...)
		out, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return totalSynced, fmt.Errorf("gh api graphql failed: %s (stderr: %s)", err, string(ee.Stderr))
			}
			return totalSynced, fmt.Errorf("gh api graphql execution failed: %w", err)
		}

		var resp graphQLResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return totalSynced, fmt.Errorf("failed parsing gh response: %w", err)
		}

		nodes := resp.Data.Viewer.Repositories.Nodes
		for _, n := range nodes {
			var topics []string
			for _, tn := range n.RepositoryTopics.Nodes {
				if tn.Topic.Name != "" {
					topics = append(topics, tn.Topic.Name)
				}
			}
			topicsJSON, _ := json.Marshal(topics)

			var lang string
			if n.PrimaryLanguage != nil {
				lang = n.PrimaryLanguage.Name
			}

			var desc string
			if n.Description != nil {
				desc = *n.Description
			}

			var homepage string
			if n.HomepageURL != nil {
				homepage = *n.HomepageURL
			}

			var pushedAt string
			if n.PushedAt != nil {
				pushedAt = *n.PushedAt
			}

			var defaultBranch, defaultBranchOID string
			if n.DefaultBranchRef != nil {
				defaultBranch = n.DefaultBranchRef.Name
				if n.DefaultBranchRef.Target != nil {
					defaultBranchOID = n.DefaultBranchRef.Target.OID
				}
			}

			var licSPDX, licName string
			if n.LicenseInfo != nil {
				licName = n.LicenseInfo.Name
				if n.LicenseInfo.SpdxID != nil {
					licSPDX = *n.LicenseInfo.SpdxID
				}
			}

			_, err := stmt.ExecContext(ctx,
				n.ID, n.Name, n.NameWithOwner, n.Owner.Login, n.Owner.URL, n.URL,
				homepage, desc, string(topicsJSON), lang,
				boolToInt(n.IsPrivate), boolToInt(n.IsFork), boolToInt(n.IsArchived),
				boolToInt(n.IsTemplate), boolToInt(n.IsDisabled),
				n.CreatedAt, n.UpdatedAt, pushedAt,
				defaultBranch, defaultBranchOID,
				n.StargazerCount, n.Watchers.TotalCount, n.ForkCount,
				n.Issues.TotalCount, n.PullRequests.TotalCount,
				licSPDX, licName,
			)
			if err != nil {
				return totalSynced, fmt.Errorf("failed upserting repo %s: %w", n.NameWithOwner, err)
			}
			totalSynced++
		}

		if !resp.Data.Viewer.Repositories.PageInfo.HasNextPage {
			break
		}
		afterCursor = resp.Data.Viewer.Repositories.PageInfo.EndCursor
	}

	return totalSynced, nil
}

type ExportedRepo struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	FullName         string   `json:"full_name"`
	OwnerLogin       string   `json:"owner_login"`
	OwnerURL         string   `json:"owner_url"`
	HTMLURL          string   `json:"html_url"`
	Homepage         *string  `json:"homepage"`
	Description      *string  `json:"description"`
	Topics           []string `json:"topics"`
	Language         *string  `json:"language"`
	Private          bool     `json:"private"`
	Fork             bool     `json:"fork"`
	Archived         bool     `json:"archived"`
	Template         bool     `json:"template"`
	Disabled         bool     `json:"disabled"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
	PushedAt         *string  `json:"pushed_at"`
	DefaultBranch    string   `json:"default_branch"`
	DefaultBranchOID string   `json:"default_branch_oid"`
	StargazersCount  int      `json:"stargazers_count"`
	WatchersCount    int      `json:"watchers_count"`
	ForksCount       int      `json:"forks_count"`
	OpenIssuesCount  int      `json:"open_issues_count"`
	OpenPRsCount     int      `json:"open_prs_count"`
	LicenseSPDX      *string  `json:"license_spdx"`
	LicenseName      *string  `json:"license_name"`
}

// ExportGitHubRepos exports all synced GitHub repos to nichsedge.github.io/data/github_repos_all.json.
func ExportGitHubRepos(ctx context.Context, db *sql.DB, outputPath string) (int, error) {
	if outputPath == "" {
		home, _ := os.UserHomeDir()
		outputPath = filepath.Join(home, "Projects", "nichsedge.github.io", "data", "github_repos_all.json")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT
			repo_id, name, full_name, owner_login, owner_url, html_url,
			homepage, description, topics, language, private, fork,
			archived, template, disabled, created_at, updated_at, pushed_at,
			default_branch, default_branch_oid, stargazers_count, watchers_count,
			forks_count, open_issues_count, open_prs_count, license_spdx, license_name
		FROM github_repositories
		ORDER BY pushed_at DESC, updated_at DESC
	`)
	if err != nil {
		return 0, fmt.Errorf("failed querying github_repositories: %w", err)
	}
	defer rows.Close()

	var records []ExportedRepo
	for rows.Next() {
		var (
			r                                          ExportedRepo
			homepage, desc, topicsVal, lang, pushed    sql.NullString
			priv, frk, arch, tmpl, dis                 int
			defBranch, defBranchOID                    sql.NullString
			licSPDX, licName                           sql.NullString
		)

		err := rows.Scan(
			&r.ID, &r.Name, &r.FullName, &r.OwnerLogin, &r.OwnerURL, &r.HTMLURL,
			&homepage, &desc, &topicsVal, &lang, &priv, &frk,
			&arch, &tmpl, &dis, &r.CreatedAt, &r.UpdatedAt, &pushed,
			&defBranch, &defBranchOID, &r.StargazersCount, &r.WatchersCount,
			&r.ForksCount, &r.OpenIssuesCount, &r.OpenPRsCount, &licSPDX, &licName,
		)
		if err != nil {
			return 0, fmt.Errorf("failed scanning row: %w", err)
		}

		if homepage.Valid && homepage.String != "" {
			r.Homepage = &homepage.String
		}
		if desc.Valid && desc.String != "" {
			r.Description = &desc.String
		}
		if lang.Valid && lang.String != "" {
			r.Language = &lang.String
		}
		if pushed.Valid && pushed.String != "" {
			r.PushedAt = &pushed.String
		}
		r.DefaultBranch = defBranch.String
		r.DefaultBranchOID = defBranchOID.String
		if licSPDX.Valid && licSPDX.String != "" {
			r.LicenseSPDX = &licSPDX.String
		}
		if licName.Valid && licName.String != "" {
			r.LicenseName = &licName.String
		}

		r.Private = priv == 1
		r.Fork = frk == 1
		r.Archived = arch == 1
		r.Template = tmpl == 1
		r.Disabled = dis == 1

		r.Topics = []string{}
		if topicsVal.Valid && topicsVal.String != "" {
			_ = json.Unmarshal([]byte(topicsVal.String), &r.Topics)
			if r.Topics == nil {
				r.Topics = []string{}
			}
		}

		records = append(records, r)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return 0, err
	}

	payloadObj := struct {
		GeneratedAtUTC string         `json:"generated_at_utc"`
		Source         string         `json:"source"`
		Affiliations   []string       `json:"affiliations"`
		Count          int            `json:"count"`
		Repos          []ExportedRepo `json:"repos"`
	}{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Source:         "ierp (events.db)",
		Affiliations:   []string{"OWNER", "COLLABORATOR", "ORGANIZATION_MEMBER"},
		Count:          len(records),
		Repos:          records,
	}

	payload, err := json.MarshalIndent(payloadObj, "", "  ")
	if err != nil {
		return 0, err
	}

	if err := os.WriteFile(outputPath, payload, 0644); err != nil {
		return 0, err
	}

	return len(records), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
