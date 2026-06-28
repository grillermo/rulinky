# frozen_string_literal: true

require "test_helper"
require "webmock/minitest"

class SlackOtpNotifierTest < ActiveSupport::TestCase
  WEBHOOK_URL = "https://hooks.slack.com/services/T000/B000/XXXX"

  def setup
    WebMock.reset!
    @original_webhook = ENV["SLACK_WEBHOOK_URL"]
    ENV["SLACK_WEBHOOK_URL"] = WEBHOOK_URL
  end

  def teardown
    ENV["SLACK_WEBHOOK_URL"] = @original_webhook
    User.delete_all
  end

  test "send_otp posts the OTP code to the Slack webhook" do
    user = User.create!(email: "mail@example.com")
    user.update!(otp_code: "123456", otp_expires_at: 10.minutes.from_now)

    stub = stub_request(:post, WEBHOOK_URL)
      .with(
        headers: { "Content-Type" => "application/json" },
        body: { text: "Your OTP for rulinky 123456" }.to_json
      )
      .to_return(status: 200, body: "ok")

    SlackOtpNotifier.send_otp(user)

    assert_requested(stub)
  end

  test "send_otp no-ops when webhook is not configured" do
    ENV["SLACK_WEBHOOK_URL"] = nil
    user = User.create!(email: "mail@example.com")
    user.update!(otp_code: "123456", otp_expires_at: 10.minutes.from_now)

    SlackOtpNotifier.send_otp(user)

    assert_not_requested(:post, WEBHOOK_URL)
  end
end
