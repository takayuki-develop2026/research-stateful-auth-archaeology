class Admin::Atlaskernel::AiRuntimeController < ApplicationController
  def index
    @project_id = current_project_id
    @task_type = params[:task_type].presence || "ocr"
    @preset = normalize_preset(params[:preset].presence || "balanced")

    @catalog = AiRuntimeClient.get_catalog(project_id: @project_id)
    @selected = build_selected_from_params(params, @preset)
    @generated_input_preview = params[:generated_input_preview].to_s
    @uploaded_evidence = {}
    @error = nil
  rescue AiRuntimeClient::Error => e
    @project_id ||= current_project_id
    @task_type ||= params[:task_type].presence || "ocr"
    @preset ||= normalize_preset(params[:preset].presence || "balanced")
    @catalog = default_empty_catalog
    @selected = build_selected_from_params(params, @preset)
    @generated_input_preview = params[:generated_input_preview].to_s
    @uploaded_evidence = {}
    @error = { message: e.message, status: e.status, body: e.body }
  end

  def create
    @project_id = params[:project_id].presence || current_project_id
    @task_type = params[:task_type].presence || "ocr"
    @preset = normalize_preset(params[:preset].presence || "balanced")

    trace_id = params[:trace_id].to_s.strip
    trace_id = fallback_trace_id if trace_id.blank?

    pipeline_version = params[:pipeline_version].presence || "v22.1"
    policy_version_str = params[:policy_version_str].presence || "v1"

    engine_selection = build_engine_selection_payload(params, @preset)
    inputs, uploaded_evidence, generated_input_preview = build_inputs_payload_for_create!(
      params: params,
      project_id: @project_id,
      task_type: @task_type
    )

    payload = {
      project_id: @project_id,
      trace_id: trace_id,
      task_type: @task_type,
      pipeline_version: pipeline_version,
      policy_version_str: policy_version_str,
      preset: @preset,
      engine_selection: engine_selection,
      inputs: inputs
    }

    res = AiRuntimeClient.create_run(payload)

    run_id =
      res["id"] ||
      res["run_id"] ||
      res.dig("run", "id") ||
      res.dig("task", "id")

    if run_id.blank?
      raise AiRuntimeClient::Error.new(
        "AI Runtime API response did not include run id",
        body: res.to_json
      )
    end

    redirect_to "/admin/dashboard/atlaskernel/ai-runtime/#{run_id}?project_id=#{CGI.escape(@project_id)}"
  rescue AiRuntimeClient::Error, ArgumentError => e
    @catalog = safe_catalog(@project_id)
    @selected = build_selected_from_params(params, @preset)
    @generated_input_preview ||= params[:generated_input_preview].to_s
    @uploaded_evidence ||= uploaded_evidence_or_empty(defined?(uploaded_evidence) ? uploaded_evidence : nil)
    @error = {
      message: e.message,
      status: (e.respond_to?(:status) ? e.status : 422),
      body: (e.respond_to?(:body) ? e.body : "")
    }

    render :index, status: :unprocessable_entity
  rescue JSON::ParserError => e
    @catalog = safe_catalog(@project_id)
    @selected = build_selected_from_params(params, @preset)
    @generated_input_preview = params[:inputs_json].to_s
    @uploaded_evidence = {}
    @error = {
      message: "inputs_json is invalid JSON: #{e.message}",
      status: 422,
      body: params[:inputs_json].to_s
    }

    render :index, status: :unprocessable_entity
  end

  def show
    @project_id = current_project_id
    @run_id = params[:id].to_s

    @data = AiRuntimeClient.get_run(@run_id, project_id: @project_id)
    @error = nil

    @run = @data["run"] || {}
    @task = @data["task"] || {}
    @model_runs = @data["model_runs"] || []
    @results = @data["results"] || []
    @normalized_result = @data["normalized_result"] || {}
    @review_queue_item = @data["review_queue_item"] || {}
    @downstream_handoffs = @data["downstream_handoffs"] || []
    @evidence_refs = @data["evidence_refs"] || []
  rescue AiRuntimeClient::Error => e
    @error = { message: e.message, status: e.status, body: e.body }
    reset_show_state!
  end

  private

  def current_project_id
    params[:project_id].presence || ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"].presence || "default"
  end

  def normalize_preset(v)
    case v.to_s
    when "fast" then "fast"
    when "high_accuracy" then "high_accuracy"
    when "custom" then "custom"
    else "balanced"
    end
  end

  def fallback_trace_id
    "airt-#{Time.now.to_i}-#{SecureRandom.hex(6)}"
  end

  def safe_catalog(project_id)
    AiRuntimeClient.get_catalog(project_id: project_id)
  rescue StandardError
    default_empty_catalog
  end

  def default_empty_catalog
    {
      "preprocess" => [],
      "ocr" => [],
      "docparse" => [],
      "embedding" => [],
      "vision" => [],
      "llm" => []
    }
  end

  def build_selected_from_params(params, preset)
    {
      "preset" => preset,
      "preprocess" => Array(params[:preprocess]).reject(&:blank?),
      "ocr" => Array(params[:ocr]).reject(&:blank?),
      "docparse" => Array(params[:docparse]).reject(&:blank?),
      "embedding" => Array(params[:embedding]).reject(&:blank?),
      "vision" => Array(params[:vision]).reject(&:blank?),
      "llm" => Array(params[:llm]).reject(&:blank?)
    }
  end

  def build_engine_selection_payload(params, preset)
    selected = build_selected_from_params(params, preset)

    engine_arrays = selected.values_at("preprocess", "ocr", "docparse", "embedding", "vision", "llm")
    return { preset: preset } if engine_arrays.all?(&:blank?)

    {
      preset: selected["preset"],
      preprocess: selected["preprocess"],
      ocr: selected["ocr"],
      docparse: selected["docparse"],
      embedding: selected["embedding"],
      vision: selected["vision"],
      llm: selected["llm"]
    }
  end

  def build_inputs_payload_for_create!(params:, project_id:, task_type:)
    input_mode = params[:input_mode].presence || "manual_json"
    input_role = params[:input_role].presence || "primary"

    case input_mode
    when "manual_json"
      inputs = build_inputs_from_manual_json!(params[:inputs_json].to_s)
      preview = JSON.pretty_generate(inputs)
      [inputs, {}, preview]

    when "upload"
      file = params[:input_file]
      raise ArgumentError, "input_file is required for upload mode" if file.blank?

      uploaded = upload_input_evidence!(
        project_id: project_id,
        task_type: task_type,
        input_role: input_role,
        file: file,
        original_filename: params[:original_filename].to_s
      )

      input = build_single_input_from_evidence_hash!(
        uploaded,
        input_role: input_role
      )

      preview = JSON.pretty_generate([input])
      [[input], uploaded, preview]

    when "existing_evidence"
      evidence_id = params[:existing_evidence_id].to_s.strip
      raise ArgumentError, "existing_evidence_id is required for existing_evidence mode" if evidence_id.blank?

      summary = AiRuntimeClient.get_evidence_summary(
        project_id: project_id,
        evidence_id: evidence_id
      )

      input = build_single_input_from_evidence_hash!(
        summary,
        input_role: input_role
      )

      preview = JSON.pretty_generate([input])
      [[input], summary, preview]

    else
      raise ArgumentError, "unsupported input_mode: #{input_mode}"
    end
  end

  def build_inputs_from_manual_json!(raw)
    raise ArgumentError, "inputs_json is required for manual_json mode" if raw.to_s.strip.blank?

    parsed = JSON.parse(raw)
    raise ArgumentError, "inputs_json must be a JSON array" unless parsed.is_a?(Array)

    parsed
  end

  def upload_input_evidence!(project_id:, task_type:, input_role:, file:, original_filename:)
    content_type = detect_content_type(file)
    validate_upload_for_task_type!(task_type: task_type, content_type: content_type)

    AiRuntimeClient.upload_input_evidence(
      project_id: project_id,
      task_type: task_type,
      input_role: input_role,
      file: file,
      original_filename: original_filename.presence || file.original_filename,
      content_type: content_type
    )
  end

  def detect_content_type(file)
    file.content_type.to_s.presence || Marcel::MimeType.for(file, name: file.original_filename).to_s
  end

  def validate_upload_for_task_type!(task_type:, content_type:)
    return if task_type == "ocr" && allowed_ocr_content_type?(content_type)
    return if task_type == "vision" && allowed_vision_content_type?(content_type)
    return if %w[fulltext_extract preprocess docparse embedding llm].include?(task_type)

    raise ArgumentError, "input file content_type #{content_type.inspect} is not allowed for task_type=#{task_type}"
  end

  def allowed_ocr_content_type?(content_type)
    %w[
      application/pdf
      image/png
      image/jpeg
      image/webp
      text/plain
    ].include?(content_type)
  end

  def allowed_vision_content_type?(content_type)
    %w[
      image/png
      image/jpeg
      image/webp
      application/pdf
    ].include?(content_type)
  end

  def build_single_input_from_evidence_hash!(hash, input_role:)
    evidence_id = hash["evidence_id"] || hash[:evidence_id] || hash["id"] || hash[:id]
    raise ArgumentError, "evidence response did not include evidence_id" if evidence_id.blank?

    {
      "input_role" => input_role,
      "evidence_id" => evidence_id,
      "seq" => 1,
      "sha256" => hash["sha256"] || hash[:sha256],
      "kind" => hash["kind"] || hash[:kind] || hash["content_type"] || hash[:content_type],
      "bytes" => hash["bytes"] || hash[:bytes] || hash["size_bytes"] || hash[:size_bytes]
    }.compact
  end

  def uploaded_evidence_or_empty(value)
    value.presence || {}
  end

  def reset_show_state!
    @data = {}
    @run = {}
    @task = {}
    @model_runs = []
    @results = []
    @normalized_result = {}
    @review_queue_item = {}
    @downstream_handoffs = []
    @evidence_refs = []
  end
end