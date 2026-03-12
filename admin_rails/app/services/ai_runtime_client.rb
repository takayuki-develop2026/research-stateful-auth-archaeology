require "net/http"
require "json"
require "uri"
require "securerandom"

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

    def upload_input_evidence(project_id:, task_type:, input_role:, file:, original_filename:, content_type:)
      instance.upload_input_evidence(
        project_id: project_id,
        task_type: task_type,
        input_role: input_role,
        file: file,
        original_filename: original_filename,
        content_type: content_type
      )
    end

    def get_evidence_summary(project_id:, evidence_id:)
      instance.get_evidence_summary(project_id: project_id, evidence_id: evidence_id)
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
  def create_run(payload = {})
    post_json(runs_path, payload)
  end

  # GET /api/admin/atlaskernel/ai-runtime/runs/:id
  def get_run(id, project_id:)
    get_json(with_query("#{runs_path}/#{id}", { project_id: project_id }))
  end

  # POST /api/admin/atlaskernel/ai-runtime/evidence/upload
  #
  # 想定 response:
  # {
  #   "evidence_id": 123,
  #   "kind": "application/pdf",
  #   "bytes": 102400,
  #   "sha256": "...",
  #   "filename": "sample.pdf"
  # }
  def upload_input_evidence(project_id:, task_type:, input_role:, file:, original_filename:, content_type:)
    raise ArgumentError, "file is required" if file.blank?
    raise ArgumentError, "project_id is required" if project_id.to_s.strip.empty?

    upload_io = extract_upload_io(file)
    filename = safe_filename(original_filename.presence || upload_io[:original_filename].presence || "upload.bin")
    mime = content_type.to_s.strip.presence || upload_io[:content_type].presence || "application/octet-stream"

    form = {
      "project_id" => project_id.to_s,
      "task_type" => task_type.to_s,
      "input_role" => input_role.to_s,
      "original_filename" => filename,
      "file" => {
        filename: filename,
        content_type: mime,
        data: upload_io[:data]
      }
    }

    post_multipart(upload_evidence_path, form)
  end

  # GET /api/admin/atlaskernel/ai-runtime/evidence/:id
  #
  # 想定 response:
  # {
  #   "evidence_id": 123,
  #   "kind": "application/pdf",
  #   "bytes": 102400,
  #   "sha256": "...",
  #   "filename": "sample.pdf"
  # }
  def get_evidence_summary(project_id:, evidence_id:)
    raise ArgumentError, "project_id is required" if project_id.to_s.strip.empty?
    raise ArgumentError, "evidence_id is required" if evidence_id.to_s.strip.empty?

    get_json(
      with_query(
        evidence_summary_path(evidence_id),
        { project_id: project_id }
      )
    )
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

  def evidence_base_path
    ENV.fetch("AI_RUNTIME_ADMIN_EVIDENCE_BASE_PATH", "#{base_path}/evidence")
  end

  def upload_evidence_path
    ENV.fetch("AI_RUNTIME_ADMIN_EVIDENCE_UPLOAD_PATH", "#{evidence_base_path}/upload")
  end

  def evidence_summary_path(evidence_id)
    "#{ENV.fetch('AI_RUNTIME_ADMIN_EVIDENCE_SHOW_BASE_PATH', evidence_base_path)}/#{evidence_id}"
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
    http = build_http(uri)

    req = klass.new(uri.request_uri)
    apply_default_headers(req)
    req["Content-Type"] = "application/json" if payload
    req.body = JSON.dump(payload) if payload

    execute_and_parse_json(http, req)
  rescue Error
    raise
  rescue => e
    raise Error.new("Request failed: #{e.class}: #{e.message}")
  end

  def post_multipart(path, form_data)
    uri = URI.parse(@base_url + path)
    http = build_http(uri)

    boundary = "----ai-runtime-#{SecureRandom.hex(16)}"
    req = Net::HTTP::Post.new(uri.request_uri)
    apply_default_headers(req)
    req["Content-Type"] = "multipart/form-data; boundary=#{boundary}"
    req.body = build_multipart_body(boundary, form_data)

    execute_and_parse_json(http, req)
  rescue Error
    raise
  rescue => e
    raise Error.new("Multipart request failed: #{e.class}: #{e.message}")
  end

  def build_http(uri)
    http = Net::HTTP.new(uri.host, uri.port)
    http.open_timeout = 5
    http.read_timeout = 60
    http.use_ssl = (uri.scheme == "https")
    http.set_debug_output($stderr) if ENV["AI_RUNTIME_API_DEBUG"] == "1"
    http
  end

  def apply_default_headers(req)
    req["Accept"] = "application/json"
    req["X-Admin-Key"] = @admin_key unless @admin_key.empty?
  end

  def execute_and_parse_json(http, req)
    res = http.request(req)
    body = res.body.to_s

    if res.code.to_i >= 400
      raise Error.new("AI Runtime API error", status: res.code.to_i, body: body)
    end

    return {} if body.strip.empty?

    JSON.parse(body)
  rescue JSON::ParserError
    raise Error.new("Invalid JSON response", status: res&.code&.to_i, body: body)
  end

  def extract_upload_io(file)
    if file.respond_to?(:tempfile) && file.tempfile
      io = file.tempfile
      io.binmode if io.respond_to?(:binmode)
      io.rewind if io.respond_to?(:rewind)
      data = io.read
      io.rewind if io.respond_to?(:rewind)

      {
        data: data,
        original_filename: file.respond_to?(:original_filename) ? file.original_filename.to_s : nil,
        content_type: file.respond_to?(:content_type) ? file.content_type.to_s : nil
      }
    elsif file.respond_to?(:read)
      file.binmode if file.respond_to?(:binmode)
      file.rewind if file.respond_to?(:rewind)
      data = file.read
      file.rewind if file.respond_to?(:rewind)

      {
        data: data,
        original_filename: nil,
        content_type: nil
      }
    else
      raise ArgumentError, "unsupported upload object"
    end
  end

  def safe_filename(name)
    n = name.to_s.strip
    n = "upload.bin" if n.empty?
    n.gsub(/[^\w.\-+]/, "_")
  end

  def build_multipart_body(boundary, form_data)
    body = +""
    form_data.each do |name, value|
      if value.is_a?(Hash) && value.key?(:data)
        filename = safe_filename(value[:filename] || "upload.bin")
        content_type = value[:content_type].to_s.strip.presence || "application/octet-stream"
        data = value[:data]
        data = data.to_s.b

        body << "--#{boundary}\r\n"
        body << %(Content-Disposition: form-data; name="#{escape_quotes(name)}"; filename="#{escape_quotes(filename)}"\r\n)
        body << "Content-Type: #{content_type}\r\n"
        body << "\r\n"
        body << data
        body << "\r\n"
      else
        body << "--#{boundary}\r\n"
        body << %(Content-Disposition: form-data; name="#{escape_quotes(name)}"\r\n)
        body << "\r\n"
        body << value.to_s
        body << "\r\n"
      end
    end
    body << "--#{boundary}--\r\n"
    body
  end

  def escape_quotes(value)
    value.to_s.gsub('"', '\"')
  end
end