require "net/http"
require "json"
require "uri"

module TrustLedger
  module AdminApi
    class Error < StandardError
      attr_reader :status, :body

      def initialize(message, status: nil, body: nil)
        super(message)
        @status = status
        @body = body
      end
    end

    class Client
      class << self
        # “singleton もどき”
        def instance
          @instance ||= new
        end

        # class methods
        def get_health = instance.get_health
        def list_webhook_events(params = {}) = instance.list_webhook_events(params)
        def get_webhook_event(event_id) = instance.get_webhook_event(event_id)
        def replay_webhook_event(event_id) = instance.replay_webhook_event(event_id)
        def get_global_kpis(params = {}) = instance.get_global_kpis(params)
        def get_shop_kpis(params = {}) = instance.get_shop_kpis(params)
        def search_postings(params = {}) = instance.search_postings(params)
        def get_posting_detail(posting_id) = instance.get_posting_detail(posting_id)
        def list_missing_sales(params = {}) = instance.list_missing_sales(params)
        def replay_sale(params = {}) = instance.replay_sale(params)
        def list_shops(params = {}) = instance.list_shops(params)

        # ★ mode を追加（API必須）
        def update_shop_payment_provider(shop_id, provider:, mode: "row") =
          instance.update_shop_payment_provider(shop_id, provider: provider, mode: mode)
      end

      def initialize(
        base_url: ENV.fetch("TRUSTLEDGER_ADMIN_API_BASE_URL", "http://localhost:8081"),
        admin_key: ENV.fetch("TRUSTLEDGER_ADMIN_X_ADMIN_KEY", "")
      )
        @base_url = base_url.to_s.sub(%r{/\z}, "")
        @admin_key = admin_key.to_s
      end

      # TrustLedger: Health
      def get_health
        get_json("/api/admin/trustledger/health")
      end

      # Webhook Events
      def list_webhook_events(params = {})
        get_json(with_query("/api/admin/trustledger/webhooks/events", params))
      end

      def get_webhook_event(event_id)
        get_json("/api/admin/trustledger/webhooks/events/#{event_id}")
      end

      def replay_webhook_event(event_id)
        post_json("/api/admin/trustledger/webhooks/events/#{event_id}/replay", {})
      end

      # KPI
      def get_global_kpis(params = {})
        get_json(with_query("/api/admin/trustledger/kpis/global", params))
      end

      def get_shop_kpis(params = {})
        get_json(with_query("/api/admin/trustledger/kpis/shops", params))
      end

      # Postings
      def search_postings(params = {})
        get_json(with_query("/api/admin/trustledger/postings", params))
      end

      def get_posting_detail(posting_id)
        get_json("/api/admin/trustledger/postings/#{posting_id}")
      end

      # Reconciliation
      def list_missing_sales(params = {})
        get_json(with_query("/api/admin/trustledger/reconciliation/missing-sales", params))
      end

      def replay_sale(params = {})
        post_json("/api/admin/trustledger/replay/sale", params)
      end

      # Shops (Provider Settings)
      def list_shops(params = {})
        get_json(with_query("/api/admin/trustledger/shops", params))
      end

      def get_shop(shop_id) = instance.get_shop(shop_id)

      def get_shop(shop_id)
        get_json("/api/admin/trustledger/shops/#{shop_id}")
      end

      # ★ mode を追加（API必須）
      def update_shop_payment_provider(shop_id, provider:, mode: "row")
        post_json(
          "/api/admin/trustledger/shops/payment-provider",
          { mode: mode, shop_id: shop_id, provider: provider }
        )
      end

      private

      def with_query(path, params)
        q = URI.encode_www_form(params.compact)
        q.empty? ? path : "#{path}?#{q}"
      end

      def get_json(path)
        request_json(Net::HTTP::Get, path)
      end

      def post_json(path, payload)
        request_json(Net::HTTP::Post, path, payload: payload)
      end

      def request_json(klass, path, payload: nil)
        uri  = URI.parse(@base_url + path)
        http = Net::HTTP.new(uri.host, uri.port)
        http.open_timeout = 5
        http.read_timeout = 10
        http.use_ssl = (uri.scheme == "https")

        # HTTP 生ログ（1=on）
        http.set_debug_output($stderr) if ENV["TRUSTLEDGER_ADMIN_API_DEBUG"] == "1"

        req = klass.new(uri.request_uri)
        req["Accept"] = "application/json"
        req["X-Admin-Key"] = @admin_key unless @admin_key.empty?

        if payload
          req["Content-Type"] = "application/json"
          req.body = JSON.dump(payload)
        end

        res  = http.request(req)
        body = res.body.to_s

        if res.code.to_i >= 400
          raise Error.new("Admin API error", status: res.code.to_i, body: body)
        end

        return {} if body.strip.empty? # 204/empty

        JSON.parse(body)
      rescue Error
        raise
      rescue JSON::ParserError
        raise Error.new("Invalid JSON response", status: res&.code&.to_i, body: body)
      rescue => e
        raise Error.new("Request failed: #{e.class}: #{e.message}")
      end
    end
  end
end