# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-cloud-pubsub>=2.30.0",
# ]
# ///
"""
Pub/Sub Mock Event Publisher.
Publishes a simulated receipt event to the gmail-expenses topic to test
end-to-end processing without needing live Gmail authorization.
"""

import os
import sys
import json
from google.cloud import pubsub_v1

PROJECT_ID = os.environ.get("GCP_PROJECT", "ichsanul-dev")
TOPIC_PATH = f"projects/{PROJECT_ID}/topics/gmail-expenses"

def main():
    publisher = pubsub_v1.PublisherClient()
    
    mock_payload = {
        "mock_expense": {
            "sender": "no-reply@gopay.co.id",
            "subject": "Pembayaran Berhasil di Kopi Kenangan",
            "amount": 38000.0,
            "currency": "IDR",
            "merchant": "Kopi Kenangan",
            "category": "Food & Beverage",
            "payment_method": "GoPay",
            "snippet": "Pembayaran senilai Rp 38.000 menggunakan GoPay di Kopi Kenangan berhasil pada 03 Nov 2026. No Transaksi: GP-881299."
        }
    }
    
    data = json.dumps(mock_payload).encode("utf-8")
    print(f"📤 Publishing mock expense event to: {TOPIC_PATH}...")
    future = publisher.publish(TOPIC_PATH, data)
    message_id = future.result()
    print(f"✅ Published message ID: {message_id}")

if __name__ == "__main__":
    main()
