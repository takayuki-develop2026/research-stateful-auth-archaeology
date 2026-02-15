class Admin::Atlaskernel::RunArtifactsController < ApplicationController
  def index
    @kind = RunArtifacts::ArtifactKind.normalize(params[:kind])
    @q    = params[:q].to_s.strip
    @q    = "" if @q.length > 200

    @known_kinds = RunArtifacts::ArtifactKind.known_kinds

    # pagination params (optional)
    limit  = (params[:limit].presence || "50").to_i
    limit  = [[limit, 1].max, 200].min
    cursor = params[:cursor].to_s.strip
    cursor = nil if cursor.empty?

    @data = TrustLedger::AdminApi::Client.list_run_artifacts(
      kind: @kind.presence,
      q: @q.presence,
      limit: limit,
      cursor: cursor
    )
  rescue TrustLedger::AdminApi::Error => e
    @error = { message: e.message, status: e.status, body: e.body }
    @data = {}
  end
end
