module Admin
  module Atlaskernel
    class DiscoveryOpsController < ApplicationController
      layout "admin"

      def index
        redirect_to action: :stale, project_id: resolved_project_id
      end

      def stale
        load_list(mode: "stale")
        render :stale
      end

      def retry
        load_list(mode: "retry")
        render :retry
      end

      def apply_retry
        load_list(mode: "apply_retry")
        render :apply_retry
      end

      def archived
        load_list(mode: "archived")
        render :archived
      end

      def show
        @project_id = resolved_project_id
        @id = params[:id].to_s

        if @project_id.blank?
          @error = { message: "project_id is required (set project_id in nav)", status: nil, body: nil }
          @candidate = nil
          @events = {}
          return
        end

        @candidate = TrustLedger::AdminApi::Client.get_discovery_candidate(@id)
        @events = TrustLedger::AdminApi::Client.get_discovery_candidate_events(@id, { limit: 50 })
      rescue TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
        @candidate = nil
        @events = {}
      end

      # -----------------------------
      # Ops (POST)
      # -----------------------------

      def requeue_review
        post_action!("requeue-review", reason: params[:reason])
      end

      def retry_now
        post_action!("retry")
      end

      def apply_retry_now
        post_action!("apply-retry")
      end

      def archive
        post_action!("archive", reason: params[:reason].presence || "manual")
      end

      def unarchive
        post_action!("unarchive", reason: params[:reason].presence || "manual")
      end

      private

      def load_list(mode:)
        @project_id = resolved_project_id
        @mode = mode

        if @project_id.blank?
          @error = { message: "project_id is required (set project_id in nav)", status: nil, body: nil }
          @items = {}
          return
        end

        p = {
          project_id: @project_id,
          mode: @mode,
          type: params[:type].presence,
          status: params[:status].presence,
          q: params[:q].presence,
          only_due: params[:only_due].presence,
          min_queue_age_days: params[:min_queue_age_days].presence,
          from: params[:from].presence,
          to: params[:to].presence,
          limit: (params[:limit].presence || 100),
        }.compact

        @items = TrustLedger::AdminApi::Client.list_discovery_ops(p)
      rescue TrustLedger::AdminApi::Error => e
        @error = { message: e.message, status: e.status, body: e.body }
        @items = {}
      end

      def resolved_project_id
        pid = params[:project_id].to_s.strip
        pid = ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"].to_s.strip if pid.empty?
        pid.presence
      end

      def post_action!(action, payload = {})
        @project_id = resolved_project_id
        if @project_id.blank?
          redirect_back fallback_location: stale_fallback_url,
                        alert: "project_id is required"
          return
        end

        id = params[:id].to_s
        trace_id = "railsops-#{Time.now.to_i}-#{SecureRandom.hex(4)}"
        body = payload.merge(project_id: @project_id, trace_id: trace_id)

        TrustLedger::AdminApi::Client.post_discovery_candidate_action(id, action, body)

        redirect_back fallback_location: stale_fallback_url,
                      notice: "OK: #{action}"
      rescue TrustLedger::AdminApi::Error => e
        redirect_back fallback_location: stale_fallback_url,
                      alert: "API Error: #{e.message} (status=#{e.status})"
      end

      def stale_fallback_url
        pid = resolved_project_id
        q = pid ? "?project_id=#{pid}" : ""
        "/admin/dashboard/atlaskernel/discovery-ops/stale#{q}"
      end
    end
  end
end