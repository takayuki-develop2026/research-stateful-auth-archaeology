require "net/http"
require "uri"
require "json"

module Admin
  module Decisioncore
    class HealthController < ApplicationController
      def show
        base = ENV.fetch("DECISIONCORE_BASE_URL", "http://localhost:9023")
        uri = URI.parse("#{base}/health")

        res = Net::HTTP.get_response(uri)
        body = res.body.to_s
        parsed = JSON.parse(body) rescue { "raw" => body }

        @status = res.code.to_i
        @body = parsed
        @base_url = base
      rescue => e
        @status = 0
        @body = { "error" => e.class.name, "message" => e.message }
        @base_url = ENV.fetch("DECISIONCORE_BASE_URL", "http://localhost:9023")
      end
    end
  end
end