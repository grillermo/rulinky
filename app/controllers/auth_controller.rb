# frozen_string_literal: true

class AuthController < ApplicationController
  skip_before_action :require_login, raise: false

  def new
    render inertia: "Auth/New"
  end

  def create
    email = params[:email].to_s.downcase.strip

    unless email.match?(URI::MailTo::EMAIL_REGEXP)
      redirect_to new_auth_path, inertia: { errors: { email: "Enter a valid email" } }
      return
    end

    user = User.find_or_create_by!(email: email)
    user.generate_otp!
    Rails.logger.info "[OTP] #{email} -> #{user.otp_code}"
    if Rails.env.production?
      OtpMailer.send_otp(user)
    end
    session[:auth_email] = email

    redirect_to verify_auth_path
  end

  def verify_form
    return redirect_to new_auth_path unless session[:auth_email]

    render inertia: "Auth/Verify", props: { email: session[:auth_email] }
  end

  def verify
    user = User.find_by(email: session[:auth_email])

    if user&.verify_otp!(params[:code].to_s.strip)
      session.delete(:auth_email)
      sign_in(user)
      redirect_to root_path
    else
      redirect_to verify_auth_path, inertia: { errors: { code: "Invalid or expired code" } }
    end
  end

  def destroy
    sign_out(current_user) if user_signed_in?
    redirect_to new_auth_path
  end
end
