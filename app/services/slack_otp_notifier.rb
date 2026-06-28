# frozen_string_literal: true

require "net/http"
require "uri"
require "json"

class SlackOtpNotifier
  def self.send_otp(user)
    webhook = ENV["SLACK_WEBHOOK_URL"]
    return if webhook.blank?

    uri = URI(webhook)
    body = { text: "Your OTP for rulinky #{user.otp_code}" }

    Net::HTTP.post(uri, body.to_json, "Content-Type" => "application/json")
  end
end
