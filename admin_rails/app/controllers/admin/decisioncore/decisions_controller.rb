require "securerandom"

module Admin
  module Decisioncore
    class DecisionsController < ApplicationController
      def index
        @project_id = params[:project_id].presence
        @base_url = ENV.fetch("DECISIONCORE_BASE_URL", "http://decisioncore_server:9023")
        @trace_id = request.headers["X-Trace-Id"].presence || "rails_decisions_#{Time.now.to_i}_#{SecureRandom.hex(3)}"

        if @project_id.blank?
          @error = { status: 0, body: { "error" => "project_id_required" } }
          @decisions = []
          return
        end

        # NOTE: DecisionCore側に list API がまだ無いので、DB読みにせず「まずはリンク導線のみ」…だと弱い。
        # なので v23側に list API を追加するのが理想だが、薄皮としては "showページへID入力" 方式でも可。
        #
        # ここでは「DBから最近の decision_id を拾う」等は RailsがSoTを持たない方針に反するので避ける。
        # 代替：decision_id を入力して show に飛べる導線＋直近IDをフォームで運用。
        #
        # → ただしあなたは最小2ページで完了したいので、ここでは "decision_id 検索" を主にする。

        @decisions = []
      end

      def show
        @project_id = params[:project_id].presence
        @base_url = ENV.fetch("DECISIONCORE_BASE_URL", "http://decisioncore_server:9023")
        @trace_id = request.headers["X-Trace-Id"].presence || "rails_decision_show_#{Time.now.to_i}_#{SecureRandom.hex(3)}"
        @id = params[:id].to_s

        if @project_id.blank?
          @error = { status: 0, body: { "error" => "project_id_required" } }
          return
        end

        # NOTE: DecisionCore側に GET decision detail API がまだ無い（現状は approve/applyのみ）なので、
        # 薄皮としては DB参照にせず、まず approve/apply の結果と actions を画面に表示する導線を作る。
        #
        # ここでは "actions一覧" は DBで見たいところだが、SoTはAPI中心のため、次ステップで DecisionCoreに
        # GET /v1/projects/{project_id}/actions?decision_id= を足すのが正しい。
        #
        @info = { status: 200, body: { "note" => "detail read API not yet implemented; use approve/apply buttons", "decision_id" => @id } }
      end

      def approve
        project_id = params[:project_id].presence
        id = params[:id].to_s
        base = ENV.fetch("DECISIONCORE_BASE_URL", "http://decisioncore_server:9023")
        trace = request.headers["X-Trace-Id"].presence || "rails_approve_#{Time.now.to_i}_#{SecureRandom.hex(3)}"
        client = DecisionCore::Client.new(base_url: base, trace_id: trace)

        res = client.post("/v1/projects/#{project_id}/decisions/#{id}/approve", { decided_by_id: "rails_admin" })
        redirect_to "/admin/dashboard/decisioncore/decisions/#{id}?project_id=#{project_id}", notice: "approve: status=#{res[:status]}"
      end

      def apply
        project_id = params[:project_id].presence
        id = params[:id].to_s
        base = ENV.fetch("DECISIONCORE_BASE_URL", "http://decisioncore_server:9023")
        trace = request.headers["X-Trace-Id"].presence || "rails_apply_#{Time.now.to_i}_#{SecureRandom.hex(3)}"
        client = DecisionCore::Client.new(base_url: base, trace_id: trace)

        # 最小：run_id は Rails側で発行せず、フォーム入力にする（薄皮）
        run_id = params[:run_id].to_s.strip
        target_id = params[:target_evidence_asset_id].to_i
        plan_id = params[:plan_evidence_asset_id].to_i

        res = client.post("/v1/projects/#{project_id}/decisions/#{id}/apply", {
          run_id: run_id,
          action_type: "publish_http",
          action_scope: "managed",
          target_evidence_asset_id: target_id,
          plan_evidence_asset_id: plan_id,
          budget_currency: "usd_micros",
          budget_estimate_amount: 1000
        })

        redirect_to "/admin/dashboard/decisioncore/decisions/#{id}?project_id=#{project_id}", notice: "apply: status=#{res[:status]} body=#{res[:body].inspect}"
      end
    end
  end
end