# frozen_string_literal: true

require "test_helper"

class UserTest < ActiveSupport::TestCase
  def teardown
    User.delete_all
  end

  test "requires a valid, unique email" do
    User.create!(email: "a@example.com")
    dup = User.new(email: "a@example.com")
    assert_not dup.valid?

    bad = User.new(email: "not-an-email")
    assert_not bad.valid?
  end

  test "generates an api_token on create" do
    user = User.create!(email: "token@example.com")
    assert user.api_token.present?
  end

  test "generate_otp! sets a 6-digit code and a future expiry" do
    user = User.create!(email: "otp@example.com")
    user.generate_otp!
    assert_match(/\A\d{6}\z/, user.otp_code)
    assert user.otp_expires_at.future?
  end

  test "verify_otp! succeeds for the right code, confirms, and clears the code" do
    user = User.create!(email: "verify@example.com")
    user.generate_otp!
    code = user.otp_code

    assert user.verify_otp!(code)
    assert user.confirmed_at.present?
    assert_nil user.otp_code
    assert_nil user.otp_expires_at
  end

  test "verify_otp! fails for a wrong or expired code" do
    user = User.create!(email: "bad@example.com")
    user.generate_otp!

    assert_not user.verify_otp!("000000") if user.otp_code != "000000"

    user.update!(otp_expires_at: 1.minute.ago)
    assert_not user.verify_otp!(user.otp_code)
  end
end
