# frozen_string_literal: true

require "test_helper"

class OtpSignInTest < ActionDispatch::IntegrationTest
  def teardown
    Link.delete_all
    User.delete_all
  end

  test "full sign-in flow: OTP read from console log" do
    log_output = StringIO.new
    original_logger = Rails.logger
    Rails.logger = Logger.new(log_output)

    post auth_path, params: { email: "e2e@example.com" }

    Rails.logger = original_logger

    otp = log_output.string.match(/\[OTP\] e2e@example\.com -> (\d{6})/)[1]

    post "/auth/verify", params: { code: otp }
    assert_redirected_to root_path

    user = User.find_by(email: "e2e@example.com")
    assert user.confirmed_at.present?
    assert_nil user.otp_code
  end
end
