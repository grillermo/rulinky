# frozen_string_literal: true

class User < ApplicationRecord
  devise :rememberable, :trackable, :timeoutable

  has_secure_token :api_token
  has_many :links, dependent: :destroy

  validates :email, presence: true,
                    uniqueness: { case_sensitive: false },
                    format: { with: URI::MailTo::EMAIL_REGEXP }

  OTP_TTL = 10.minutes

  def generate_otp!
    update!(
      otp_code: format("%06d", SecureRandom.random_number(1_000_000)),
      otp_expires_at: OTP_TTL.from_now
    )
  end

  def otp_valid?(code)
    otp_code.present? &&
      otp_expires_at&.future? &&
      ActiveSupport::SecurityUtils.secure_compare(otp_code, code.to_s)
  end

  def verify_otp!(code)
    return false unless otp_valid?(code)

    update!(otp_code: nil, otp_expires_at: nil, confirmed_at: confirmed_at || Time.current)
    true
  end
end
