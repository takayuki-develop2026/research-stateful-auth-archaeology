require "date"

module Admin
  module Trustledger
    class DashboardController < ApplicationController
      def show
        client = ::TrustLedger::AdminApi::Client.new

        # Health
        @health = client.get_health

        # Recent failed webhook events (last 7d, top 10)
        from = (Date.today - 7).strftime("%Y-%m-%d")
        to   = Date.today.strftime("%Y-%m-%d")

        @recent_failed_webhooks =
          client.list_webhook_events(
            from: from,
            to: to,
            limit: "10",       # ✅ per_page ではなく limit
            status: "error",   # ✅ failed ではなく error
          )["items"] || []

        # ReviewQueue pending count
        begin
          rq = client.list_review_queue(status: "pending", limit: "1", offset: "0")
          @review_pending_total = (rq["total"] || rq.dig("page","total") || 0).to_i
        rescue
          @review_pending_total = nil
        end
      rescue ::TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
      end
    end
  end
end