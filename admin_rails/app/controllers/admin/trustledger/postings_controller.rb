require "date"

module Admin
  module Trustledger
    class PostingsController < ApplicationController
      def index
  client = ::TrustLedger::AdminApi::Client.new

  default_from = (Date.today - 30).strftime("%Y-%m-%d")
  default_to   = Date.today.strftime("%Y-%m-%d")

  shop_id = params[:shop_id].presence

  api_params = {
    from: params[:from].presence || default_from,
    to: params[:to].presence || default_to,
    currency: params[:currency].presence,
    posting_type: params[:posting_type].presence,

    # ✅ 絞り込みは「どっちか1つ」に統一（Laravel仕様に合わせて）
    shop_ids: shop_id ? [shop_id.to_i] : nil,

    payment_id: params[:payment_id].presence,
    order_id: params[:order_id].presence,
    source_event_id: params[:source_event_id].presence,
    source_provider: params[:provider].presence,
    limit: (params[:limit].presence || 50).to_i,
    cursor: params[:cursor].presence,
  }.compact

  # q は最後に上書き
  q_value =
    params[:payment_id].presence ||
    params[:order_id].presence ||
    params[:source_event_id].presence ||
    params[:q].presence

  api_params[:q] = q_value.to_s.strip if q_value.present?

  Rails.logger.info("[🔥postings] params_shop_id=#{params[:shop_id].inspect} api_params=#{api_params.inspect}")

  @data  = client.search_postings(api_params)
  @items = @data["items"] || @data["postings"] || []
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