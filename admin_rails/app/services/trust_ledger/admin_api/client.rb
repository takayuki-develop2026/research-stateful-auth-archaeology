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
        def instance
          @instance ||= new
        end

        # -----------------------------
        # TrustLedger (Go/SpringBoot) APIs
        # -----------------------------
        def get_health = instance.get_health

        # Webhook
        def list_webhook_events(params = {}) = instance.list_webhook_events(params)
        def get_webhook_event(event_id) = instance.get_webhook_event(event_id)
        def replay_webhook_event(event_id) = instance.replay_webhook_event(event_id)

        # KPI
        def get_global_kpis(params = {}) = instance.get_global_kpis(params)
        def get_shop_kpis(params = {}) = instance.get_shop_kpis(params)

        # Postings
        def search_postings(params = {}) = instance.search_postings(params)
        def get_posting_detail(posting_id) = instance.get_posting_detail(posting_id)

        # Reconciliation
        def list_missing_sales(params = {}) = instance.list_missing_sales(params)
        def replay_sale(params = {}) = instance.replay_sale(params)


        # Discovery Ops (AtlasKernel)
        def list_discovery_ops(params = {}) = instance.list_discovery_ops(params)
        def get_discovery_candidate(id) = instance.get_discovery_candidate(id)
        def get_discovery_candidate_events(id, params = {}) = instance.get_discovery_candidate_events(id, params)
        def post_discovery_candidate_action(id, action, payload = {}) = instance.post_discovery_candidate_action(id, action, payload)


        # Shops
        def list_shops(params = {}) = instance.list_shops(params)
        def get_shop(shop_id)
          shops_res = list_shops
          shops =
            if shops_res.is_a?(Hash)
              shops_res["shops"] || shops_res["items"] || shops_res["data"] || []
            else
              shops_res
            end

          shop = shops.find { |s| s["id"].to_i == shop_id.to_i }
          provider = get_shop_payment_provider(shop_id)

          {
            "shop" => shop,
            "payment_provider" => provider,
          }
        end

        def get_shop_payment_provider(shop_id)
          instance.get_shop_payment_provider(shop_id)
        end

        def update_shop_payment_provider(shop_id, provider:, mode: "row") =
          instance.update_shop_payment_provider(shop_id, provider: provider, mode: mode)

        # Review Queue
        def list_review_queue(params = {}) = instance.list_review_queue(params)
        def get_review_queue_item(id) = instance.get_review_queue_item(id)
        def decide_review_queue_item(id, payload = {}) = instance.decide_review_queue_item(id, payload)

        # ProviderIntel
        def get_providerintel_document(id) = instance.get_providerintel_document(id)
        def get_providerintel_diff(id) = instance.get_providerintel_diff(id)

        # ProviderIntel: Catalog Sources
        def list_catalog_sources(params = {}) = instance.list_catalog_sources(params)
        def get_catalog_source(source_id) = instance.get_catalog_source(source_id)
        def upsert_catalog_source(payload = {}) = instance.upsert_catalog_source(payload)
        def run_catalog_source(source_id) = instance.run_catalog_source(source_id)

        # Budget
        def gate_and_reserve_budget(payload = {}) = instance.gate_and_reserve_budget(payload)

        # -----------------------------
        # AtlasKernel (Laravel) APIs
        # -----------------------------
        def list_run_artifacts(params = {}) = instance.list_run_artifacts(params)
      end

      # TRUSTLEDGER_ADMIN_API_BASE_URL:
      #   TrustLedger Admin API (Go/SpringBoot) base
      # ATLASKERNEL_ADMIN_API_BASE_URL:
      #   AtlasKernel Admin API (Laravel) base
      #
      # 例（docker compose 内）:
      #   TRUSTLEDGER_ADMIN_API_BASE_URL=http://payment_core:8081
      #   ATLASKERNEL_ADMIN_API_BASE_URL=http://nginx
      #
      def initialize(
        base_url: ENV.fetch("TRUSTLEDGER_ADMIN_API_BASE_URL", "http://localhost:8081"),
        atlas_base_url: ENV.fetch(
          "ATLASKERNEL_ADMIN_API_BASE_URL",
          ENV.fetch("TRUSTLEDGER_ADMIN_API_BASE_URL", "http://localhost:8081")
        ),
        admin_key: ENV.fetch("TRUSTLEDGER_ADMIN_X_ADMIN_KEY", "")
      )
        @base_url = normalize_base_url(base_url)
        @atlas_base_url = normalize_base_url(atlas_base_url)
        @admin_key = admin_key.to_s
      end

      # -----------------------------
      # TrustLedger APIs (Go/SpringBoot)
      # -----------------------------
      def get_health
        get_json("/api/admin/trustledger/health")
      end

      # Webhook
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

      # Shops
      def list_shops(params = {})
        get_json(with_query("/api/admin/trustledger/shops", params))
      end

      def get_shop(shop_id)
        get_json("/api/admin/trustledger/shops/#{shop_id}")
      end

      def get_shop_payment_provider(shop_id)
        get_json("/api/admin/trustledger/shops/#{shop_id}/payment-provider")
      end

      def update_shop_payment_provider(shop_id, provider:, mode: "row")
        post_json(
          "/api/admin/trustledger/shops/payment-provider",
          { mode: mode, shop_id: shop_id, provider: provider }
        )
      end

      # Review Queue
      def review_queue_path
        ENV.fetch("TRUSTLEDGER_ADMIN_REVIEW_QUEUE_PATH", "/api/admin/review-queue")
      end

      def list_review_queue(params = {})
        get_json(with_query(review_queue_path, params))
      end

      def get_review_queue_item(id)
        get_json("#{review_queue_path}/#{id}")
      end

      def decide_review_queue_item(id, payload = {})
        post_json("#{review_queue_path}/#{id}/decide", payload)
      end

      # ProviderIntel
      def providerintel_path
        ENV.fetch("TRUSTLEDGER_ADMIN_PROVIDERINTEL_PATH", "/api/admin/providerintel")
      end

      def get_providerintel_document(id)
        get_json("#{providerintel_path}/documents/#{id}")
      end

      def get_providerintel_diff(id)
        get_json("#{providerintel_path}/diffs/#{id}")
      end

      # Catalog Sources
      def providerintel_sources_path
        ENV.fetch("TRUSTLEDGER_ADMIN_PROVIDERINTEL_SOURCES_PATH", "#{providerintel_path}/sources")
      end

      def list_catalog_sources(params = {})
        get_json(with_query(providerintel_sources_path, params))
      end

      def get_catalog_source(source_id)
        get_json("#{providerintel_sources_path}/#{source_id}")
      end

      def upsert_catalog_source(payload = {})
        post_json(providerintel_sources_path, payload)
      end

      def run_catalog_source(source_id)
        post_json("#{providerintel_sources_path}/#{source_id}/run", {})
      end

      # Budget
      def gate_and_reserve_budget(payload = {})
        post_json("/api/admin/trustledger/budgets/reserve", payload)
      end

      # -----------------------------
      # AtlasKernel APIs (Laravel)
      # -----------------------------
      #
      # ✅ Run Artifacts は Laravel 側へ必ず流す
      #
      def list_run_artifacts(params = {})
        get_json_atlas(with_query("/api/admin/atlaskernel/run-artifacts", params))
      end

      # -----------------------------
# Discovery Ops (AtlasKernel)
# -----------------------------
def discovery_ops_path
  ENV.fetch("ATLASKERNEL_ADMIN_DISCOVERY_OPS_PATH", "/api/admin/atlaskernel/discovery-ops")
end

def list_discovery_ops(params = {})
  get_json_atlas(with_query("#{discovery_ops_path}/candidates", params))
end

def get_discovery_candidate(id)
  get_json_atlas("#{discovery_ops_path}/candidates/#{id}")
end

def get_discovery_candidate_events(id, params = {})
  get_json_atlas(with_query("#{discovery_ops_path}/candidates/#{id}/events", params))
end

def post_discovery_candidate_action(id, action, payload = {})
  post_json_atlas("#{discovery_ops_path}/candidates/#{id}/#{action}", payload)
end

      private

      def normalize_base_url(v)
        v.to_s.sub(%r{/\z}, "")
      end

      def with_query(path, params)
        q = URI.encode_www_form((params || {}).compact)
        q.empty? ? path : "#{path}?#{q}"
      end

      # TrustLedger base_url へ
      def get_json(path)
        request_json(Net::HTTP::Get, path, base_url: @base_url)
      end

      def post_json(path, payload)
        request_json(Net::HTTP::Post, path, payload: payload, base_url: @base_url)
      end

      # AtlasKernel base_url へ
      def get_json_atlas(path)
        request_json(Net::HTTP::Get, path, base_url: @atlas_base_url)
      end

      def post_json_atlas(path, payload)
        request_json(Net::HTTP::Post, path, payload: payload, base_url: @atlas_base_url)
      end

      def request_json(klass, path, payload: nil, base_url:)
  uri  = URI.parse(base_url + path)
  http = Net::HTTP.new(uri.host, uri.port)
  http.open_timeout = 5
  http.read_timeout = 10
  http.use_ssl = (uri.scheme == "https")

  http.set_debug_output($stderr) if ENV["TRUSTLEDGER_ADMIN_API_DEBUG"] == "1"

  req = klass.new(uri.request_uri)
  req["Accept"] = "application/json"
  req["X-Admin-Key"] = @admin_key unless @admin_key.empty?

  if payload
    req["Content-Type"] = "application/json"
    req.body = JSON.dump(payload)
  end

  res = nil
  body = ""

  res  = http.request(req)
  body = res.body.to_s

  if res.code.to_i >= 400
    raise Error.new("Admin API error", status: res.code.to_i, body: body)
  end

  return {} if body.strip.empty?
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