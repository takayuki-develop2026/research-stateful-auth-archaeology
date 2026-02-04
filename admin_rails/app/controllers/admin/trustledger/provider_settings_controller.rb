require "date"

module Admin
  module Trustledger
    class ProviderSettingsController < ApplicationController
      before_action :load_filters, only: %i[index update]

      def index
        @data  = admin_api.list_shops(api_params)
        @items = @data["items"] || []
Rails.logger.info("[ProviderSettings.index] sample=#{@items.first.inspect}") if @items.first
        meta_total, meta_pages, meta_page =
          if @data["page"].is_a?(Hash)
            [
              @data.dig("page", "total"),
              @data.dig("page", "pages"),
              @data.dig("page", "page"),
            ]
          else
            [
              @data["total"],
              @data["pages"],
              @data["page"],
            ]
          end

        @total = meta_total.to_i
        @pages = meta_pages.to_i
        @pages = (@total.to_f / @limit).ceil if @pages <= 0
        @pages = 1 if @pages <= 0

        @page = meta_page.to_i if meta_page.present?
        @page = 1 if @page <= 0
      rescue ::TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
        @items = []
        @total = 0
        @pages = 1
        @page  = 1
      end

      def update
        Rails.logger.info(
          "[ProviderSettings.update] mode=#{params[:mode]} " \
          "shop_id=#{params[:shop_id]} provider=#{params[:provider]} " \
          "shop_ids=#{params[:shop_ids].inspect}"
        )

        mode = params[:mode].to_s

        if mode == "bulk"
          provider = normalize_provider!(params[:provider])
          ids = normalize_ids(params[:shop_ids])

          if ids.empty?
            flash[:alert] = "更新対象の shop が選択されていません。"
            return redirect_back fallback_location: index_path
          end

          ok = 0
          ng = []

          ids.each do |id|
            begin
              admin_api.update_shop_payment_provider(id, provider: provider, mode: "bulk")
              ok += 1
            rescue ::TrustLedger::AdminApi::Error => e
              ng << { shop_id: id, status: e.status, body: (e.body.presence || e.message) }
            end
          end

          if ng.empty?
            flash[:notice] = "一括更新 OK: #{ok}件"
          else
            flash[:alert]  = "一括更新: OK=#{ok} / NG=#{ng.size}（詳細はログ）"
            Rails.logger.warn("[ProviderSettings.bulk] failed=#{ng.inspect}")
          end

          return redirect_back fallback_location: index_path
        end

        shop_id  = params[:shop_id].to_i
        provider = normalize_provider!(params[:provider])

        if shop_id <= 0
          flash[:alert] = "shop_id が不正です。"
          return redirect_back fallback_location: index_path
        end

  admin_api.update_shop_payment_provider(shop_id, provider: provider, mode: "row")

# ✅ 直後に read-back（Clientにあるメソッド名に合わせて）
shop = admin_api.get_shop(shop_id)
Rails.logger.info("[ProviderSettings.update] readback shop_id=#{shop_id} payment_provider_now=#{shop["payment_provider"].inspect} updated_at=#{shop["updated_at"].inspect}")
Rails.logger.info("[ProviderSettings.update] readback shop_id=#{shop_id} payment_provider_now=#{shop["payment_provider"] || shop.dig("item","payment_provider")}")

flash[:notice] = "更新しました（shop_id=#{shop_id}, provider=#{provider}）"
redirect_back fallback_location: index_path

      rescue ArgumentError => e
        flash[:alert] = e.message
        redirect_back fallback_location: index_path
      rescue ::TrustLedger::AdminApi::Error => e
        msg = e.body.presence || e.message
        flash[:alert] = "更新失敗: #{e.status || "?"} #{msg}"
        redirect_back fallback_location: index_path
      end

      private

      # ✅ Client は毎回 new しない
      def admin_api
        ::TrustLedger::AdminApi::Client.instance
      end

      def index_path
        "/admin/dashboard/trustledger/provider-settings"
      end

      # ✅ params を読むのは全部ここ（= class body 事故の温床をゼロにする）
      def load_filters
        @q = params[:q].to_s.strip
        @q = nil if @q.empty?

        @status   = normalize_status(params[:status])
        @provider = normalize_provider(params[:provider])

        @limit = params[:limit].presence.to_i
        @limit = 50  if @limit <= 0
        @limit = 200 if @limit > 200

        @page = params[:page].presence.to_i
        @page = 1 if @page <= 0
      end

      def api_params
        {
          q: @q,
          status: @status,
          provider: @provider,
          limit: @limit,
          page: @page,
          sort: (params[:sort].presence || "updated_desc"),
        }.compact
      end

      def normalize_status(raw)
        s = raw.to_s.strip
        return nil if s.empty? || s == "all"
        return s if %w[active inactive].include?(s)
        nil
      end

      def normalize_provider(raw)
        p = raw.to_s.strip
        return nil if p.empty? || p == "all"
        return p if %w[stripe adyen].include?(p)
        nil
      end

      def normalize_provider!(raw)
        p = raw.to_s.strip
        unless %w[stripe adyen].include?(p)
          raise ArgumentError, "provider が不正です（stripe / adyen）"
        end
        p
      end

      def normalize_ids(v)
        arr =
          case v
          when Array  then v
          when String then v.split(",")
          else []
          end

        arr.map { |x| x.to_s.strip }
           .select { |x| x.match?(/\A\d+\z/) }
           .map(&:to_i)
           .select { |x| x > 0 }
           .uniq
      end
    end
  end
end