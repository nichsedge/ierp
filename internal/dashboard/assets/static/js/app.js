/**
 * iERP Dashboard — Reactive Application Engine (Petite-Vue)
 * Pure standard architecture: Zero build steps, offline-first.
 */

(function () {
  const debounceTimers = {};
  const activeObservers = {};

  const typeIcons = {
    book: '📖 Book',
    film: '🎬 Film',
    anime: '🎌 Anime',
    manga: '📚 Manga',
    drama: '🎭 Drama'
  };

  const app = {
    activeTab: 'overview',
    commerceSubtab: 'payments',
    
    // Toast notification
    toast: {
      show: false,
      message: ''
    },
    showToast(msg, duration = 2500) {
      this.toast.message = msg;
      this.toast.show = true;
      if (this._toastTimer) clearTimeout(this._toastTimer);
      this._toastTimer = setTimeout(() => {
        this.toast.show = false;
      }, duration);
    },

    // Modal state
    modalEvent: null,
    async showEventModal(eventId) {
      try {
        const res = await fetch('/api/events/' + eventId);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        this.modalEvent = await res.json();
        if (window.history.state?.modal !== true) {
          window.history.pushState({ modal: true }, '');
        }
      } catch (err) {
        console.error('Failed to load event detail:', err);
        this.showToast('Failed to load event details');
      }
    },
    closeModal() {
      if (this.modalEvent) {
        this.modalEvent = null;
        if (window.history.state?.modal === true) {
          window.history.back();
        }
      }
    },

    // Overview Stats
    statsLoading: false,
    stats: {
      total_events: 0,
      total_contacts: 0,
      total_links: 0,
      total_media: 0,
      runway: '-',
      radar_overdue: '-',
      net_cash: '-',
      net_pos: '-',
      top_tags: [],
      monthly_counts: [],
      daily_counts: {},
      top_places: [],
      media_summary: {},
      cashflow: []
    },
    heatmapCells: [],
    heatmapMonths: [],
    heatmapTotalLabel: '',
    monthlyCountsChart: [],
    cashflowChartData: [],

    // Paginated collection states
    collections: {
      events: {
        endpoint: '/api/events',
        items: [],
        total: 0,
        limit: 25,
        offset: 0,
        sort: 'start_date',
        dir: 'desc',
        search: '',
        tag: '',
        place: '',
        from_date: '',
        to_date: '',
        loading: false,
        hasMore: true
      },
      contacts: {
        endpoint: '/api/contacts',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'event_count',
        dir: 'desc',
        search: '',
        source: '',
        loading: false,
        hasMore: true
      },
      media: {
        endpoint: '/api/media',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'id',
        dir: 'desc',
        search: '',
        mediaType: '',
        loading: false,
        hasMore: true
      },
      vendors: {
        endpoint: '/api/vendors',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'favorite',
        dir: 'desc',
        search: '',
        category: '',
        categories: [],
        favoriteOnly: false,
        loading: false,
        hasMore: true
      },
      links: {
        endpoint: '/api/links',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'category',
        dir: 'asc',
        search: '',
        category: '',
        categories: [],
        loading: false,
        hasMore: true
      },
      payments: {
        endpoint: '/api/payment-accounts',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'category',
        dir: 'asc',
        search: '',
        loading: false,
        hasMore: true
      },
      referrals: {
        endpoint: '/api/referrals',
        items: [],
        total: 0,
        limit: 24,
        offset: 0,
        sort: 'category',
        dir: 'asc',
        search: '',
        loading: false,
        hasMore: true
      }
    },

    // Non-paginated views
    projects: {
      items: [],
      total: 0,
      status: '',
      loading: false
    },
    decisions: {
      items: [],
      total: 0,
      status: '',
      loading: false
    },
    radar: {
      items: [],
      total: 0,
      summary: null,
      tier: '',
      overdueOnly: false,
      loading: false
    },
    lifeops: {
      runway: '-',
      burn: '-',
      liquid: '-',
      networth: '-',
      maintenance: [],
      reviews: [],
      loading: false
    },

    // Lifecycle Init
    init() {
      const VALID_TABS = ['overview', 'events', 'contacts', 'projects', 'decisions', 'radar', 'lifeops', 'media', 'vendors', 'links', 'commerce'];
      const hashTab = window.location.hash ? window.location.hash.replace('#', '') : '';
      const initialTab = VALID_TABS.includes(hashTab) ? hashTab : 'overview';
      this.switchTab(initialTab, false);

      this.$nextTick(() => {
        this.setupObservers();
      });

      window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && this.modalEvent) {
          this.closeModal();
        }
      });

      window.addEventListener('popstate', () => {
        if (this.modalEvent) {
          this.modalEvent = null;
          return;
        }
        const currentHash = window.location.hash ? window.location.hash.replace('#', '') : '';
        if (VALID_TABS.includes(currentHash) && currentHash !== this.activeTab) {
          this.switchTab(currentHash, false);
        }
      });
    },

    // Navigation
    switchTab(tabName, updateHash = true) {
      this.activeTab = tabName;
      if (updateHash && window.location.hash !== '#' + tabName) {
        if (window.history.replaceState) {
          window.history.replaceState(null, '', '#' + tabName);
        } else {
          window.location.hash = tabName;
        }
      }

      if (tabName === 'overview') {
        this.loadStats();
      } else if (['events', 'contacts', 'media', 'vendors', 'links'].includes(tabName)) {
        if (this.collections[tabName].items.length === 0) {
          this.fetchTabItems(tabName, false);
        }
      } else if (tabName === 'commerce') {
        const sub = this.commerceSubtab;
        if (this.collections[sub].items.length === 0) {
          this.fetchTabItems(sub, false);
        }
      } else if (tabName === 'projects') {
        if (this.projects.items.length === 0) this.loadProjects();
      } else if (tabName === 'decisions') {
        if (this.decisions.items.length === 0) this.loadDecisions();
      } else if (tabName === 'radar') {
        if (this.radar.items.length === 0) this.loadRadar();
      } else if (tabName === 'lifeops') {
        if (this.lifeops.maintenance.length === 0) this.loadLifeOps();
      }

      this.$nextTick(() => {
        this.setupObservers();
      });
    },

    switchCommerceSubtab(subtab) {
      this.commerceSubtab = subtab;
      if (this.collections[subtab].items.length === 0) {
        this.fetchTabItems(subtab, false);
      }
      this.$nextTick(() => {
        this.setupObservers();
      });
    },

    // Overview Stats Loader
    async loadStats() {
      this.statsLoading = true;
      try {
        const res = await fetch('/api/stats');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();

        this.stats.total_events = data.total_events || 0;
        this.stats.total_contacts = data.total_contacts || 0;
        this.stats.total_links = data.total_links || 0;
        this.stats.total_media = data.total_media || 0;
        this.stats.top_tags = data.top_tags || [];
        this.stats.top_places = data.top_places || [];
        this.stats.media_summary = data.media_summary || {};
        this.stats.daily_counts = data.daily_counts || {};
        this.stats.monthly_counts = data.monthly_counts || [];
        this.stats.cashflow = data.cashflow || [];

        if (data.finance) {
          const lq = data.finance.liquid_cash != null ? data.finance.liquid_cash : data.finance.net_cash;
          const nw = data.finance.net_worth != null ? data.finance.net_worth : data.finance.net_position;
          this.stats.net_cash = lq != null ? 'Rp' + Math.round(lq).toLocaleString('id-ID') : '-';
          this.stats.net_pos = nw != null ? 'Rp' + Math.round(nw).toLocaleString('id-ID') : '-';
          if (data.finance.runway_months != null) {
            this.stats.runway = data.finance.is_infinite ? '∞ Months' : `${data.finance.runway_months}m`;
          }
        }

        // Fetch runway
        try {
          const rwRes = await fetch('/api/runway');
          if (rwRes.ok) {
            const rw = await rwRes.json();
            this.stats.runway = rw.is_infinite ? '∞ Months' : `${rw.runway_months}m`;
          }
        } catch (e) {}

        // Fetch radar
        try {
          const rdRes = await fetch('/api/radar');
          if (rdRes.ok) {
            const rd = await rdRes.json();
            this.stats.radar_overdue = rd.summary ? `${rd.summary.total_overdue} Overdue` : '0';
          }
        } catch (e) {}

        // Compute Visualizations
        this.computeHeatmap(this.stats.daily_counts);
        this.computeMonthlyChart(this.stats.monthly_counts);
        this.computeCashflowChart(this.stats.cashflow);

      } catch (err) {
        console.error('Failed to load stats:', err);
      } finally {
        this.statsLoading = false;
      }
    },

    computeHeatmap(dailyCounts) {
      const today = new Date();
      const startDate = new Date(today);
      startDate.setDate(today.getDate() - (52 * 7));
      const dayOfWeek = startDate.getDay();
      const diffToMonday = (dayOfWeek === 0 ? -6 : 1 - dayOfWeek);
      startDate.setDate(startDate.getDate() + diffToMonday);

      let totalLoggedDays = 0;
      let totalEventsInPeriod = 0;
      const monthPositions = [];
      let lastMonth = -1;

      const current = new Date(startDate);
      let weekIndex = 0;
      const cells = [];

      while (current <= today || current.getDay() !== 1) {
        const y = current.getFullYear();
        const m = String(current.getMonth() + 1).padStart(2, '0');
        const d = String(current.getDate()).padStart(2, '0');
        const dateStr = `${y}-${m}-${d}`;

        const count = dailyCounts[dateStr] || 0;
        if (count > 0) {
          totalLoggedDays++;
          totalEventsInPeriod += count;
        }

        let level = 0;
        if (count >= 10) level = 4;
        else if (count >= 6) level = 3;
        else if (count >= 3) level = 2;
        else if (count >= 1) level = 1;

        cells.push({
          date: dateStr,
          count: count,
          level: level,
          title: `${count} event${count === 1 ? '' : 's'} on ${dateStr}`
        });

        if (current.getDay() === 1) {
          const monthNum = current.getMonth();
          if (monthNum !== lastMonth) {
            monthPositions.push({
              month: current.toLocaleString('default', { month: 'short' }),
              week: weekIndex
            });
            lastMonth = monthNum;
          }
          weekIndex++;
        }

        current.setDate(current.getDate() + 1);
      }

      this.heatmapCells = cells;
      this.heatmapMonths = monthPositions;
      this.heatmapTotalLabel = `${totalEventsInPeriod} events across ${totalLoggedDays} active days`;
    },

    computeMonthlyChart(monthlyCounts) {
      const counts = monthlyCounts || [];
      const maxCount = Math.max(...counts.map(m => m.count), 1);
      this.monthlyCountsChart = counts.slice().reverse().map(m => ({
        year_month: m.year_month,
        count: m.count,
        heightPct: Math.max((m.count / maxCount) * 100, 4)
      }));
    },

    computeCashflowChart(cashflowList) {
      if (!cashflowList || cashflowList.length === 0) {
        this.cashflowChartData = [];
        return;
      }
      const list = cashflowList.slice(0, 8).reverse();
      const maxVal = Math.max(...list.map(c => Math.max(c.income || 0, c.cost || 0, c.expected || 0)), 1);
      this.cashflowChartData = list.map(c => ({
        year_month: c.year_month,
        income: c.income || 0,
        cost: c.cost || 0,
        expected: c.expected || 0,
        incPct: Math.max(((c.income || 0) / maxVal) * 100, 3),
        costPct: Math.max(((c.cost || 0) / maxVal) * 100, 3),
        expPct: Math.max(((c.expected || 0) / maxVal) * 100, 3)
      }));
    },

    // Paginated Fetching
    async fetchTabItems(tabKey, isAppend = false) {
      const state = this.collections[tabKey];
      if (!state || state.loading) return;
      if (isAppend && !state.hasMore) return;

      state.loading = true;
      if (!isAppend) {
        state.offset = 0;
        state.items = [];
        state.hasMore = true;
      }

      try {
        const params = new URLSearchParams();
        params.set('limit', state.limit);
        params.set('offset', state.offset);
        if (state.sort) params.set('sort', state.sort);
        if (state.dir) params.set('dir', state.dir);
        if (state.search) params.set('q', state.search);

        if (tabKey === 'events') {
          if (state.tag) params.set('tag', state.tag);
          if (state.place) params.set('place', state.place);
          if (state.from_date) params.set('from', state.from_date);
          if (state.to_date) params.set('to', state.to_date);
        } else if (tabKey === 'contacts') {
          if (state.source) params.set('source', state.source);
        } else if (tabKey === 'media') {
          if (state.mediaType) params.set('type', state.mediaType);
        } else if (tabKey === 'vendors') {
          if (state.category) params.set('category', state.category);
          if (state.favoriteOnly) params.set('favorite', '1');
        } else if (tabKey === 'links') {
          if (state.category) params.set('category', state.category);
        }

        const res = await fetch(`${state.endpoint}?${params.toString()}`);
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();

        const items = Array.isArray(data) ? data : (data.items || []);
        const total = (typeof data.total === 'number') ? data.total : (isAppend ? state.items.length + items.length : items.length);

        state.total = total;
        if (isAppend) {
          state.items.push(...items);
        } else {
          state.items = items;
        }

        state.offset = state.items.length;
        state.hasMore = state.items.length < state.total && items.length > 0;

        // Dynamic categories for vendors & links
        if (!isAppend && data.categories) {
          state.categories = data.categories;
        }

      } catch (err) {
        console.error(`Failed to fetch ${tabKey}:`, err);
        this.showToast(`Failed to load ${tabKey}`);
      } finally {
        state.loading = false;
      }
    },

    fetchMore(tabKey) {
      this.fetchTabItems(tabKey, true);
    },

    resetAndFetch(tabKey) {
      this.fetchTabItems(tabKey, false);
    },

    onSearchInput(tabKey) {
      if (debounceTimers[tabKey]) clearTimeout(debounceTimers[tabKey]);
      debounceTimers[tabKey] = setTimeout(() => {
        this.resetAndFetch(tabKey);
      }, 250);
    },

    setSort(tabKey, col) {
      const c = this.collections[tabKey];
      if (!c) return;
      if (c.sort === col) {
        c.dir = c.dir === 'asc' ? 'desc' : 'asc';
      } else {
        c.sort = col;
        c.dir = 'desc';
      }
      this.resetAndFetch(tabKey);
    },

    changeLimit(tabKey, event) {
      const c = this.collections[tabKey];
      if (!c) return;
      c.limit = parseInt(event.target.value, 10) || 24;
      this.resetAndFetch(tabKey);
    },

    filterContactsSource(source) {
      this.collections.contacts.source = source;
      this.resetAndFetch('contacts');
    },

    filterLinksCategory(category) {
      this.collections.links.category = category;
      this.resetAndFetch('links');
    },

    onVendorCategoryChange(event) {
      this.collections.vendors.category = event.target.value;
      this.resetAndFetch('vendors');
    },

    onEventDateChange() {
      this.resetAndFetch('events');
    },

    // Filter Helpers
    filterByTag(tag) {
      this.switchTab('events');
      this.collections.events.tag = tag;
      this.resetAndFetch('events');
    },

    filterByPlace(place) {
      this.switchTab('events');
      this.collections.events.place = place;
      this.resetAndFetch('events');
    },

    filterByDate(dateStr) {
      this.switchTab('events');
      this.collections.events.from_date = dateStr;
      this.collections.events.to_date = dateStr;
      this.resetAndFetch('events');
    },

    filterByMediaType(type) {
      this.switchTab('media');
      this.collections.media.mediaType = type;
      this.resetAndFetch('media');
    },

    clearEventFilters() {
      const e = this.collections.events;
      e.search = '';
      e.tag = '';
      e.place = '';
      e.from_date = '';
      e.to_date = '';
      this.resetAndFetch('events');
    },

    hasActiveEventFilters() {
      const e = this.collections.events;
      return Boolean(e.search || e.tag || e.place || e.from_date || e.to_date);
    },

    toggleVendorFavFilter() {
      const v = this.collections.vendors;
      v.favoriteOnly = !v.favoriteOnly;
      this.resetAndFetch('vendors');
    },

    async toggleVendorFavorite(vendorId, event) {
      if (event) event.stopPropagation();
      try {
        const res = await fetch('/api/vendors/favorite', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: vendorId })
        });
        const data = await res.json();
        if (data.success) {
          const item = this.collections.vendors.items.find(v => v.id === vendorId);
          if (item) {
            item.favorite = data.favorite ? 1 : 0;
          }
          this.showToast(data.favorite ? 'Marked vendor as favorite ⭐' : 'Removed vendor from favorites');
        }
      } catch (err) {
        console.error('Failed to toggle vendor favorite:', err);
      }
    },

    async triggerAutoMerge() {
      if (!confirm('Run heuristic auto-merging across your contacts based on matching names, emails, and phone numbers?')) {
        return;
      }
      try {
        const res = await fetch('/api/contacts/automerge', { method: 'POST' });
        const data = await res.json();
        if (data.merged_count > 0) {
          alert(`Successfully merged ${data.merged_count} duplicate contact pairs!`);
          this.resetAndFetch('contacts');
        } else {
          alert('No duplicate contacts found to merge.');
        }
      } catch (err) {
        alert('Error during contact auto-merge: ' + err.message);
      }
    },

    copyText(text, event) {
      if (event) event.stopPropagation();
      if (!text) return;
      navigator.clipboard.writeText(text).then(() => {
        this.showToast(`Copied: "${text}"`);
      }).catch(() => {
        const ta = document.createElement('textarea');
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        this.showToast(`Copied: "${text}"`);
      });
    },

    // Projects Loader
    async loadProjects(status = '') {
      this.projects.status = status;
      this.projects.loading = true;
      try {
        const url = status ? `/api/projects?status=${encodeURIComponent(status)}` : '/api/projects';
        const res = await fetch(url);
        const data = await res.json();
        this.projects.total = data.total || 0;
        this.projects.items = data.items || [];
      } catch (err) {
        console.error('Failed to load projects:', err);
        this.showToast('Failed to load projects');
      } finally {
        this.projects.loading = false;
      }
    },

    // Decisions Loader
    async loadDecisions(status = '') {
      this.decisions.status = status;
      this.decisions.loading = true;
      try {
        const url = status ? `/api/decisions?status=${encodeURIComponent(status)}` : '/api/decisions';
        const res = await fetch(url);
        const data = await res.json();
        this.decisions.total = data.total || 0;
        this.decisions.items = data.items || [];
      } catch (err) {
        console.error('Failed to load decisions:', err);
        this.showToast('Failed to load decisions');
      } finally {
        this.decisions.loading = false;
      }
    },

    // Radar Loader
    async loadRadar(tier = '', overdueOnly = false) {
      this.radar.tier = tier;
      this.radar.overdueOnly = overdueOnly;
      this.radar.loading = true;
      try {
        let url = `/api/radar?limit=100`;
        if (tier) url += `&tier=${tier}`;
        if (overdueOnly) url += `&overdue_only=true`;
        const res = await fetch(url);
        const data = await res.json();
        this.radar.total = data.total || 0;
        this.radar.summary = data.summary || null;
        this.radar.items = data.items || [];
      } catch (err) {
        console.error('Failed to load radar:', err);
        this.showToast('Failed to load radar');
      } finally {
        this.radar.loading = false;
      }
    },

    toggleRadarOverdue() {
      this.radar.overdueOnly = !this.radar.overdueOnly;
      this.loadRadar(this.radar.tier, this.radar.overdueOnly);
    },

    filterRadarTier(tier) {
      this.radar.tier = tier;
      this.loadRadar(tier, this.radar.overdueOnly);
    },

    // Life Ops Loader
    async loadLifeOps() {
      this.lifeops.loading = true;
      try {
        const rwRes = await fetch('/api/runway');
        if (rwRes.ok) {
          const rw = await rwRes.json();
          this.lifeops.runway = rw.is_infinite ? '∞ Months' : `${rw.runway_months} Months`;
          this.lifeops.burn = 'Rp' + (rw.monthly_burn || 0).toLocaleString('id-ID');
          this.lifeops.liquid = 'Rp' + (rw.liquid_cash || 0).toLocaleString('id-ID');
          this.lifeops.networth = 'Rp' + (rw.net_worth || 0).toLocaleString('id-ID');
        }

        const mRes = await fetch('/api/maintenance?limit=10');
        if (mRes.ok) {
          const mData = await mRes.json();
          this.lifeops.maintenance = mData.items || [];
        }

        const rRes = await fetch('/api/reviews?limit=5');
        if (rRes.ok) {
          const rData = await rRes.json();
          this.lifeops.reviews = rData.items || [];
        }
      } catch (err) {
        console.error('Failed to load life ops:', err);
      } finally {
        this.lifeops.loading = false;
      }
    },

    getMediaIcon(type) {
      return typeIcons[type] || '📁';
    },

    setupObservers() {
      const keys = ['events', 'contacts', 'media', 'vendors', 'links', 'payments', 'referrals'];
      keys.forEach(k => {
        const el = document.getElementById(`${k}-sentinel`);
        if (!el) return;
        if (activeObservers[k]) {
          activeObservers[k].disconnect();
        }
        activeObservers[k] = new IntersectionObserver((entries) => {
          if (entries[0].isIntersecting) {
            const st = this.collections[k];
            if (st && st.hasMore && !st.loading && st.items.length > 0) {
              this.fetchMore(k);
            }
          }
        }, { rootMargin: '400px' });
        activeObservers[k].observe(el);
      });
    }
  };

  window.ierpApp = app;

  document.addEventListener('DOMContentLoaded', () => {
    PetiteVue.createApp(app).mount('#app');
    app.init();
  });
})();
