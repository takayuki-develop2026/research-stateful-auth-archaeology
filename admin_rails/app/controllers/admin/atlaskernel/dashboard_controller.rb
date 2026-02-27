module Admin
  module Atlaskernel
    class DashboardController < ApplicationController
      layout "admin"

      def show
        @project_id = params[:project_id].presence || ENV["ATLASKERNEL_DEFAULT_PROJECT_ID"]
      end
    end
  end
end