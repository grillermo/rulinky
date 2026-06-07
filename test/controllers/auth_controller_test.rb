# frozen_string_literal: true

require "test_helper"

class AuthControllerTest < ActionDispatch::IntegrationTest
  def teardown
    User.delete_all
  end

  test "new renders the Auth/New Inertia page" do
    get new_auth_path, headers: { "X-Inertia" => "true" }
    assert_response :ok
    assert_equal "Auth/New", JSON.parse(response.body)["component"]
  end

  test "create finds-or-creates a user, sets an OTP, and redirects to verify" do
    assert_difference "User.count", 1 do
      post auth_path, params: { email: "new@example.com" }
    end
    user = User.find_by(email: "new@example.com")
    assert user.otp_code.present?
    assert_redirected_to verify_auth_path
  end

  test "create rejects an invalid email" do
    assert_no_difference "User.count" do
      post auth_path, params: { email: "nope" }
    end
    assert_redirected_to new_auth_path
  end

  test "verify with the correct code signs in, confirms, and redirects to root" do
    post auth_path, params: { email: "login@example.com" }
    user = User.find_by(email: "login@example.com")
    code = user.otp_code

    post "/auth/verify", params: { code: code }
    assert_redirected_to root_path
    assert user.reload.confirmed_at.present?
    assert_nil user.reload.otp_code

    # session established: a guarded page now loads without redirect to auth
    get root_path
    assert_response :ok
  end

  test "verify with a wrong code redirects back to verify" do
    post auth_path, params: { email: "wrong@example.com" }
    post "/auth/verify", params: { code: "000000" }
    assert_redirected_to verify_auth_path
  end
end
