# frozen_string_literal: true

class OtpMailer < ApplicationMailer
  def send_otp(user)
    @user = user
    @code = user.otp_code
    mail(to: user.email, subject: "Your Rulinky sign-in code")
  end
end
