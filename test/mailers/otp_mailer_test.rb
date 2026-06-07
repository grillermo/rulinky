# frozen_string_literal: true

require "test_helper"

class OtpMailerTest < ActionMailer::TestCase
  def teardown
    User.delete_all
  end

  test "send_otp includes the code and is addressed to the user" do
    user = User.create!(email: "mail@example.com")
    user.update!(otp_code: "123456", otp_expires_at: 10.minutes.from_now)

    mail = OtpMailer.send_otp(user)

    assert_equal ["mail@example.com"], mail.to
    assert_match "123456", mail.body.encoded
  end
end
