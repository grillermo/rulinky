# frozen_string_literal: true

require "test_helper"
require "webmock/minitest"

class OtpMailerTest < ActiveSupport::TestCase
  MAILGUN_URL = "https://api.mailgun.net/v3/#{ENV.fetch('MAILGUN_DOMAIN', 'example.com')}/messages"

  def teardown
    User.delete_all
  end

  test "send_otp sends email with the OTP code to the user" do
    user = User.create!(email: "mail@example.com")
    user.update!(otp_code: "123456", otp_expires_at: 10.minutes.from_now)

    stub = stub_request(:post, MAILGUN_URL)
      .to_return(status: 200, body: '{"id":"<test>","message":"Queued. Thank you."}')

    OtpMailer.send_otp(user)

    assert_requested(stub)
    assert_requested(:post, MAILGUN_URL,
      body: /mail%40example\.com|mail@example\.com/)
    assert_requested(:post, MAILGUN_URL, body: /123456/)
  end
end
