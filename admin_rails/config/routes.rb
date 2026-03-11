Rails.application.routes.draw do
  get "up" => "rails/health#show", as: :rails_health_check

  namespace :admin do
    # ✅ 全体トップ
    get "/dashboard", to: "dashboard#show"

    # ✅ TrustLedger dashboard
    scope "/dashboard/trustledger", module: "trustledger" do
      get "/", to: "dashboard#show"
      get "/health", to: "health#show"

      get  "/webhook-events", to: "webhook_events#index"
      get  "/webhook-events/:event_id", to: "webhook_events#show"
      post "/webhook-events/:event_id/replay", to: "webhook_events#replay"

      get  "/kpis/global", to: "kpis#global"
      get  "/kpis/shops",  to: "kpis#shops"

      get  "/postings",             to: "postings#index"
      get  "/postings/:posting_id", to: "postings#show"

      get  "/reconciliation/missing-sales", to: "reconciliation#missing_sales"
      post "/reconciliation/replay/sale",   to: "reconciliation#replay_sale"

      get  "/provider-settings",        to: "provider_settings#index"
      post "/provider-settings/update", to: "provider_settings#update"

      get  "/providerintel/sources",                 to: "providerintel_sources#index"
      get  "/providerintel/sources/new",             to: "providerintel_sources#new"
      post "/providerintel/sources",                 to: "providerintel_sources#create"
      get  "/providerintel/sources/:source_id",      to: "providerintel_sources#show"
      get  "/providerintel/sources/:source_id/edit", to: "providerintel_sources#edit"
      post "/providerintel/sources/:source_id",      to: "providerintel_sources#update"
      post "/providerintel/sources/:source_id/run",  to: "providerintel_sources#run"

      get  "/review-queue",            to: "review_queue#index"
      get  "/review-queue/:id",        to: "review_queue#show"
      post "/review-queue/:id/decide", to: "review_queue#decide"
    end

    # ✅ AtlasKernel dashboard
    scope "/dashboard/atlaskernel", module: "atlaskernel" do
      get "/", to: "dashboard#show"

      # ✅ canonical
      get "/run-artifacts", to: "run_artifacts#index"

      # ✅ AI Runtime (v22.1)
      get  "/ai-runtime",      to: "ai_runtime#index"
      post "/ai-runtime/run",  to: "ai_runtime#create"
      get  "/ai-runtime/:id",  to: "ai_runtime#show", as: :atlaskernel_ai_runtime_run

      # ✅ Ledger ingest runs (v14)
      get "/ledger/ingest-runs", to: "ledger_ingest_runs#index"
      get "/ledger/ingest-runs/:id", to: "ledger_ingest_runs#show", as: :atlaskernel_ledger_ingest_run

      # ✅ Discovery Ops（★必ずこの scope の中）
      get  "/discovery-ops",               to: "discovery_ops#index"
      get  "/discovery-ops/stale",         to: "discovery_ops#stale"
      get  "/discovery-ops/retry",         to: "discovery_ops#retry"
      get  "/discovery-ops/apply-retry",   to: "discovery_ops#apply_retry"
      get  "/discovery-ops/archived",      to: "discovery_ops#archived"

      get  "/discovery-ops/candidates/:id", to: "discovery_ops#show", as: :discovery_ops_candidate

      post "/discovery-ops/candidates/:id/requeue-review", to: "discovery_ops#requeue_review"
      post "/discovery-ops/candidates/:id/retry",          to: "discovery_ops#retry_now"
      post "/discovery-ops/candidates/:id/apply-retry",    to: "discovery_ops#apply_retry_now"
      post "/discovery-ops/candidates/:id/archive",        to: "discovery_ops#archive"
      post "/discovery-ops/candidates/:id/unarchive",      to: "discovery_ops#unarchive"
    end

    # ✅ DecisionCore dashboard (v23)
    scope "/dashboard/decisioncore", module: "decisioncore" do
      get  "/",       to: "dashboard#show"
      get  "/health", to: "health#show"

      # ✅ v16 minimal
      get  "/decisions",             to: "decisions#index"
      get  "/decisions/:id",         to: "decisions#show"
      post "/decisions/:id/approve", to: "decisions#approve"
      post "/decisions/:id/apply",   to: "decisions#apply"
    end

    # -----------------------------
    # 旧URL互換（当面redirect）
    # -----------------------------
    get "/run-artifacts", to: redirect("/admin/dashboard/atlaskernel/run-artifacts")

    namespace :trustledger do
      get "/",       to: redirect("/admin/dashboard/trustledger")
      get "/health", to: redirect("/admin/dashboard/trustledger/health")

      get  "/webhook-events", to: redirect("/admin/dashboard/trustledger/webhook-events")
      get  "/webhook-events/:event_id", to: redirect("/admin/dashboard/trustledger/webhook-events/%{event_id}")
      post "/webhook-events/:event_id/replay", to: redirect("/admin/dashboard/trustledger/webhook-events/%{event_id}/replay")

      get "/kpis/global", to: redirect("/admin/dashboard/trustledger/kpis/global")
      get "/kpis/shops",  to: redirect("/admin/dashboard/trustledger/kpis/shops")

      get "/postings",             to: redirect("/admin/dashboard/trustledger/postings")
      get "/postings/:posting_id", to: redirect("/admin/dashboard/trustledger/postings/%{posting_id}")

      get  "/reconciliation/missing-sales", to: redirect("/admin/dashboard/trustledger/reconciliation/missing-sales")
      post "/reconciliation/replay/sale",   to: redirect("/admin/dashboard/trustledger/reconciliation/replay/sale")
    end
  end
end