require "date"

module Admin
  module Trustledger
    class PostingsController < ApplicationController
      def index
        client = ::TrustLedger::AdminApi::Client.new

        default_from = (Date.today - 30).strftime("%Y-%m-%d")
        default_to   = Date.today.strftime("%Y-%m-%d")

        api_params = {
          from: params[:from].presence || default_from,
          to: params[:to].presence || default_to,
          currency: params[:currency].presence, # 任意
          posting_type: params[:posting_type].presence,
          shop_ids: params[:shop_id].presence,
          payment_id: params[:payment_id].presence,
          order_id: params[:order_id].presence,
          source_event_id: params[:source_event_id].presence,
          source_provider: params[:provider].presence, # ✅ 追加：UIの provider をAPIへ
          q: params[:q].presence,
          limit: params[:limit].presence || 50,
          cursor: params[:cursor].presence,
        }.compact

        # ✅ Laravel側の q 仕様に合わせる：
        # - 数字だけ => order_id/payment_id/source_event_id(前方一致) を横断検索
        # - 文字列 => source_event_id の前方一致
        q_value = nil

        if params[:payment_id].present?
          q_value = params[:payment_id].to_s.strip
        elsif params[:order_id].present?
          q_value = params[:order_id].to_s.strip
        elsif params[:source_event_id].present?
          q_value = params[:source_event_id].to_s.strip
        elsif params[:q].present?
          q_value = params[:q].to_s.strip
        end

        api_params[:q] = q_value if q_value.present?

        @data = client.search_postings(api_params)
        @items = @data["items"] || @data["postings"] || [] # backend の返却差に耐える
      rescue ::TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
      end

      def show
        client = ::TrustLedger::AdminApi::Client.new
        @data = client.get_posting_detail(params[:posting_id])
      rescue ::TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
      end
    end
  end
end