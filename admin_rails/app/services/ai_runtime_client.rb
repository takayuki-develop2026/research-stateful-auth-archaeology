require "net/http"
require "json"
require "uri"

class AiRuntimeClient
  class Error < StandardError
    attr_reader :status, :body

    def initialize(message, status: nil, body: nil)
      super(message)
      @status = status
      @body = body
    end
  end

  class << self
    def instance
      @instance ||= new
    end

    def get_catalog(params = {})
      instance.get_catalog(params)
    end

    def list_runs(params = {})
      instance.list_runs(params)
    end

    def create_run(payload = {})
      instance.create_run(payload)
    end

    def get_run(id, project_id:)
      instance.get_run(id, project_id: project_id)
    end
  end

  def initialize(
    base_url: ENV.fetch(
      "AI_RUNTIME_ADMIN_API_BASE_URL",
      ENV.fetch(
        "ATLASKERNEL_ADMIN_API_BASE_URL",
        ENV.fetch("TRUSTLEDGER_ADMIN_API_BASE_URL", "http://localhost:8081")
      )
    ),
    admin_key: ENV.fetch("TRUSTLEDGER_ADMIN_X_ADMIN_KEY", "")
  )
    @base_url = normalize_base_url(base_url)
    @admin_key = admin_key.to_s
  end

  # ---------------------------------
  # Public API
  # ---------------------------------

  # GET /api/admin/atlaskernel/ai-runtime/catalog
  def get_catalog(params = {})
    get_json(with_query(catalog_path, params))
  end

  # GET /api/admin/atlaskernel/ai-runtime/runs
  def list_runs(params = {})
    get_json(with_query(runs_path, params))
  end

  # POST /api/admin/atlaskernel/ai-runtime/runs
  #
  # 想定 payload 例:
  # {
  #   project_id: "akproj_xxx",
  #   trace_id: "trace_xxx",
  #   task_type: "ocr",
  #   pipeline_version: "v22.1",
  #   policy_version_str: "v1",
  #   preset: "balanced",
  #   engine_selection: {
  #     preprocess: ["opencv_basic", "deskew_basic"],
  #     ocr: ["paddleocr"],
  #     docparse: ["pp_structure_v3"],
  #     embedding: ["openclip"],
  #     vision: ["qwen_vl"],
  #     llm: ["gemini_flash", "gpt5"]
  #   },
  #   inputs: [...]
  # }
  def create_run(payload = {})
    post_json(runs_path, payload)
  end

  # GET /api/admin/atlaskernel/ai-runtime/runs/:id
  #
  # show 画面で以下のような bundle を返す想定:
  # {
  #   "run": {...},
  #   "task": {...},
  #   "model_runs": [...],
  #   "results": [...],
  #   "normalized_result": {...},
  #   "review_queue_item": {...},
  #   "downstream_handoffs": [...],
  #   "evidence_refs": [...]
  # }
  def get_run(id, project_id:)
    get_json(with_query("#{runs_path}/#{id}", { project_id: project_id }))
  end

  # ---------------------------------
  # Path helpers
  # ---------------------------------

  def base_path
    ENV.fetch("AI_RUNTIME_ADMIN_BASE_PATH", "/api/admin/atlaskernel/ai-runtime")
  end

  def catalog_path
    ENV.fetch("AI_RUNTIME_ADMIN_CATALOG_PATH", "#{base_path}/catalog")
  end

  def runs_path
    ENV.fetch("AI_RUNTIME_ADMIN_RUNS_PATH", "#{base_path}/runs")
  end

  private

  def normalize_base_url(v)
    v.to_s.sub(%r{/\z}, "")
  end

  def with_query(path, params)
    q = URI.encode_www_form((params || {}).compact)
    q.empty? ? path : "#{path}?#{q}"
  end

  def get_json(path)
    request_json(Net::HTTP::Get, path)
  end

  def post_json(path, payload)
    request_json(Net::HTTP::Post, path, payload: payload)
  end

  def request_json(klass, path, payload: nil)
    uri = URI.parse(@base_url + path)
    http = Net::HTTP.new(uri.host, uri.port)
    http.open_timeout = 5
    http.read_timeout = 20
    http.use_ssl = (uri.scheme == "https")

    http.set_debug_output($stderr) if ENV["AI_RUNTIME_API_DEBUG"] == "1"

    req = klass.new(uri.request_uri)
    req["Accept"] = "application/json"
    req["X-Admin-Key"] = @admin_key unless @admin_key.empty?

    if payload
      req["Content-Type"] = "application/json"
      req.body = JSON.dump(payload)
    end

    res = nil
    body = ""

    res = http.request(req)
    body = res.body.to_s

    if res.code.to_i >= 400
      raise Error.new("AI Runtime API error", status: res.code.to_i, body: body)
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