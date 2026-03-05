require Rails.root.join("app/services/ledgersvc_api_client")

class Admin::Atlaskernel::LedgerIngestRunsController < ApplicationController
  PROJECT_ID_DEFAULT = "akproj_0000000000000000000"

  def index
    @project_id = (params[:project_id].presence || ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"] || PROJECT_ID_DEFAULT).to_s
    @status = params[:status].presence
    @from = params[:from].presence
    @to = params[:to].presence
    @limit = (params[:limit].presence || "50")

    client = ::LedgersvcApiClient.new(base_url: ENV.fetch("LEDGERSVC_BASE_URL", "http://ledgersvc-server:9031"))

    begin
      res = client.list_ingest_runs(
        project_id: @project_id,
        status: @status,
        from: @from,
        to: @to,
        limit: @limit
      )
      @items = (res["items"] || [])
      @error = nil
    rescue => e
      @items = []
      @error = { status: 500, body: { message: e.message } }
    end
  end

  def show
    @project_id = (params[:project_id].presence || ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"] || PROJECT_ID_DEFAULT).to_s
    @id = params[:id].to_s

    client = ::LedgersvcApiClient.new(base_url: ENV.fetch("LEDGERSVC_BASE_URL", "http://ledgersvc-server:9031"))

    begin
      @run = client.get_ingest_run(project_id: @project_id, ingest_run_id: @id)
      @error = nil
    rescue => e
      @run = nil
      @error = { status: 500, body: { message: e.message } }
      return
    end

    @evidences = []
    Array(@run["evidence_refs"]).each do |ref|
      begin
        @evidences << client.get_evidence(project_id: @project_id, evidence_ref: ref)
      rescue => e
        @evidences << { "evidence_ref" => ref, "error" => true, "error_message" => e.message }
      end
    end

    # ✅ NEW: fetch evidence JSON content (reject list)
    @evidence_contents = {}
    Array(@run["evidence_refs"]).each do |ref|
      begin
        @evidence_contents[ref] = client.get_evidence_content(project_id: @project_id, evidence_ref: ref)
      rescue => e
        @evidence_contents[ref] = "FAILED: #{e.message}"
      end
    end
  end
end