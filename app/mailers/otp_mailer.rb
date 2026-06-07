# frozen_string_literal: true

require "mailgun-ruby"

class OtpMailer
  FROM = "Rulinky <noreply@#{ENV.fetch('MAILGUN_DOMAIN', 'example.com')}>"

  def self.send_otp(user)
    Mailgun.configure { |config| config.api_key = ENV["MAILGUN_API_KEY"] }

    message = Mailgun::MessageBuilder.new
    message.from(FROM)
    message.add_recipient(:to, user.email)
    message.subject("Your Rulinky sign-in code")
    message.body_text("Your code is #{user.otp_code}.\n\nIt expires in 10 minutes.")

    Mailgun::Client.new.send_message(ENV["MAILGUN_DOMAIN"], message)
  end
end
