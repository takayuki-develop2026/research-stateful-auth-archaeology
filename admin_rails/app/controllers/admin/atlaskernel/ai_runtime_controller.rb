class Admin::Atlaskernel::AiRuntimeController < ApplicationController
  def index
    @project_id = current_project_id
    @task_type = params[:task_type].presence || "ocr"
    @preset = normalize_preset(params[:preset].presence || "balanced")

    @catalog = AiRuntimeClient.get_catalog(project_id: @project_id)
    @error = nil

    @selected = build_selected_from_params(params, @preset)
  rescue AiRuntimeClient::Error => e
    @error = { message: e.message, status: e.status, body: e.body }
    @catalog = default_empty_catalog
    @selected = build_selected_from_params(params, @preset)
  end

  def create
    project_id = current_project_id
    task_type = params[:task_type].presence || "ocr"
    preset = normalize_preset(params[:preset].presence || "balanced")

    trace_id = params[:trace_id].to_s.strip
    trace_id = fallback_trace_id if trace_id.blank?

    pipeline_version = params[:pipeline_version].presence || "v22.1"
    policy_version_str = params[:policy_version_str].presence || "v1"

    engine_selection = build_engine_selection_payload(params, preset)

    payload = {
      project_id: project_id,
      trace_id: trace_id,
      task_type: task_type,
      pipeline_version: pipeline_version,
      policy_version_str: policy_version_str,
      preset: preset,
      engine_selection: engine_selection,
      inputs: build_inputs_payload(params)
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

    redirect_to "/admin/dashboard/atlaskernel/ai-runtime/#{run_id}?project_id=#{CGI.escape(project_id)}"
  rescue AiRuntimeClient::Error => e
    @project_id = project_id
    @task_type = task_type
    @preset = preset
    @catalog = safe_catalog(project_id)
    @selected = build_selected_from_params(params, preset)
    @error = { message: e.message, status: e.status, body: e.body }

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
    @run = {}
    @task = {}
    @model_runs = []
    @results = []
    @normalized_result = {}
    @review_queue_item = {}
    @downstream_handoffs = []
    @evidence_refs = []
  end

  private

  def current_project_id
    params[:project_id].presence || ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"]
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

    if selected.values_at("preprocess", "ocr", "docparse", "embedding", "vision", "llm").all?(&:blank?)
      return { preset: preset }
    end

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

  def build_inputs_payload(params)
    raw = params[:inputs_json].to_s.strip
    return [] if raw.blank?

    parsed = JSON.parse(raw)
    parsed.is_a?(Array) ? parsed : []
  rescue JSON::ParserError
    []
  end
end