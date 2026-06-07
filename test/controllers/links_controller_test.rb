# frozen_string_literal: true

require "test_helper"

class LinksControllerTest < ActionDispatch::IntegrationTest
  def setup
    @user = User.create!(email: "links@example.com", confirmed_at: Time.current)
    ts = (Time.now.to_f * 1000).to_i
    @link = Link.create!(
      id: SecureRandom.uuid,
      url: "https://example.com",
      note: "Test note",
      read: 0,
      timestamp: ts,
      updated_at: ts,
      user: @user
    )
    sign_in_as(@user)
  end

  def teardown
    LinkContentJob.delete_all
    Link.delete_all
    User.delete_all
  end

  # Signs a user in through the real OTP flow so the session cookie is set.
  def sign_in_as(user)
    user.generate_otp!
    post auth_path, params: { email: user.email }
    post "/auth/verify", params: { code: user.reload.otp_code }
  end

  test "redirects to auth when signed out" do
    reset!
    get root_path
    assert_redirected_to new_auth_path
  end

  test "index renders Inertia component with expected props" do
    get root_path, headers: { "X-Inertia" => "true" }
    assert_response :ok
    json = JSON.parse(response.body)
    assert_equal "Links/Index", json["component"]
    assert json["props"].key?("links"), "props must include 'links'"
    assert json["props"].key?("readCount"), "props must include 'readCount'"
    assert json["props"].key?("unreadCount"), "props must include 'unreadCount'"
    assert_instance_of Array, json["props"]["links"]
    link_props = json["props"]["links"].first
    %w[id url title fullTitle read updatedAt].each do |key|
      assert link_props.key?(key), "each link must have '#{key}' key"
    end
  end

  test "index only shows the current user's links" do
    other = User.create!(email: "other@example.com", confirmed_at: Time.current)
    ts = (Time.now.to_f * 1000).to_i
    Link.create!(id: SecureRandom.uuid, url: "https://other.com",
                 read: 0, timestamp: ts, updated_at: ts, user: other)

    get root_path, headers: { "X-Inertia" => "true" }
    urls = JSON.parse(response.body)["props"]["links"].map { |l| l["url"] }
    assert_includes urls, "https://example.com"
    assert_not_includes urls, "https://other.com"
  end

  test "destroy deletes the link and redirects to root" do
    assert_difference "Link.count", -1 do
      delete link_path(@link.id)
    end
    assert_redirected_to root_path
  end

  test "read marks link as read and redirects back" do
    patch read_link_path(@link.id)
    assert_equal 1, @link.reload.read
    assert_redirected_to root_path(filter: "unread")
  end

  test "unread marks link as unread and redirects back" do
    @link.update!(read: 1)
    patch unread_link_path(@link.id)
    assert_equal 0, @link.reload.read
    assert_redirected_to root_path(filter: "read")
  end
end
