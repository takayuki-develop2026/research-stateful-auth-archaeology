# frozen_string_literal: true

require "net/http"
require "json"
require "uri"

class LedgersvcApiClient
  def initialize(base_url: ENV.fetch("LEDGERSVC_BASE_URL", "http://ledgersvc-server:9031"))
    @base_url = base_url
  end

  def list_ingest_runs(project_id:, status: nil, from: nil, to: nil, limit: 50)
    q = {}
    q["status"] = status if present?(status)
    q["from"] = from if present?(from)
    q["to"] = to if present?(to)
    q["limit"] = limit.to_i if present?(limit)

    path = "/v1/projects/#{escape(project_id)}/ledger/ingest-runs"
    get_json(path, q: q)
  end

  def get_ingest_run(project_id:, ingest_run_id:)
    path = "/v1/projects/#{escape(project_id)}/ledger/ingest-runs/#{escape(ingest_run_id)}"
    get_json(path)
  end

  def get_evidence(project_id:, evidence_ref:)
    path = "/v1/projects/#{escape(project_id)}/evidence/#{escape(evidence_ref)}"
    get_json(path)
  end

  # ✅ NEW: evidence JSON content (stored as file in ledgersvc)
  def get_evidence_content(project_id:, evidence_ref:)
    path = "/v1/projects/#{escape(project_id)}/evidence/#{escape(evidence_ref)}/content"
    get_raw(path)
  end

  private

  def get_json(path, q: nil)
    uri = URI.join(@base_url, path)
    uri.query = URI.encode_www_form(q) if q && q.any?

    res = http_get(uri)
    body = res.body.to_s
    json =
      begin
        body.empty? ? {} : JSON.parse(body)
      rescue JSON::ParserError
        { "raw" => body }
      end

    raise "API Error status=#{res.code} body=#{json}" if res.code.to_i >= 400
    json
  end

  def get_raw(path)
    uri = URI.join(@base_url, path)
    res = http_get(uri)
    body = res.body.to_s
    raise "API Error status=#{res.code} body=#{body}" if res.code.to_i >= 400
    body
  end

  def http_get(uri)
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = (uri.scheme == "https")

    req = Net::HTTP::Get.new(uri.request_uri)
    req["Accept"] = "application/json"
    req["X-Trace-Id"] = "rails_admin_#{Time.now.to_i}"

    http.request(req)
  end

  def escape(s)
    URI.encode_www_form_component(s.to_s)
  end

  def present?(v)
    !v.nil? && v.to_s.strip != ""
  end
end