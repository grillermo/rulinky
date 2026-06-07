# frozen_string_literal: true

module Api
  class BaseController < ActionController::Base
    skip_forgery_protection
    before_action :authenticate_api_user!

    private

    def authenticate_api_user!
      token = request.headers["Authorization"].to_s.sub(/\ABearer /, "").strip
      @current_user = User.find_by(api_token: token) if token.present?
      render json: { error: "Unauthorized" }, status: :unauthorized unless @current_user
    end

    def current_user
      @current_user
    end
  end
end
