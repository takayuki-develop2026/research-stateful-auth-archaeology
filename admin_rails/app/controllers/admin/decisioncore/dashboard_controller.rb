module Admin
  module Decisioncore
    class DashboardController < ApplicationController
      def show
        @project_id = params[:project_id].presence
        @decisioncore_base_url = ENV.fetch("DECISIONCORE_BASE_URL", "http://localhost:9023")
      end
    end
  end
end