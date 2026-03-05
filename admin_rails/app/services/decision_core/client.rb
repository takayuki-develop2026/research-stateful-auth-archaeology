require "net/http"
require "uri"
require "json"

module DecisionCore
  class Client
    def initialize(base_url:, trace_id:)
      @base_url = base_url
      @trace_id = trace_id
    end

    def get(path)
      uri = URI.join(@base_url, path)
      req = Net::HTTP::Get.new(uri)
      req["X-Trace-Id"] = @trace_id
      req["Accept"] = "application/json"
      do_request(uri, req)
    end

    def post(path, body = {})
      uri = URI.join(@base_url, path)
      req = Net::HTTP::Post.new(uri)
      req["X-Trace-Id"] = @trace_id
      req["Content-Type"] = "application/json"
      req.body = JSON.generate(body)
      do_request(uri, req)
    end

    private

    def do_request(uri, req)
      res = Net::HTTP.start(uri.host, uri.port) { |http| http.request(req) }
      parsed = JSON.parse(res.body) rescue { "raw" => res.body.to_s }
      { status: res.code.to_i, body: parsed }
    rescue => e
      { status: 0, body: { "error" => e.class.name, "message" => e.message } }
    end
  end
end