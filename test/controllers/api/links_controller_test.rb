# frozen_string_literal: true

require "test_helper"

class Api::LinksControllerTest < ActionDispatch::IntegrationTest
  def setup
    @user = User.create!(email: "api@example.com", confirmed_at: Time.current)
    ts = (Time.now.to_f * 1000).to_i
    @link = Link.create!(id: SecureRandom.uuid, url: "https://example.com",
                         read: 0, timestamp: ts, updated_at: ts, user: @user)
  end

  def teardown
    LinkContentJob.delete_all
    Link.delete_all
    User.delete_all
  end

  def auth_headers(token)
    { "Authorization" => "Bearer #{token}" }
  end

  test "index without a token is unauthorized" do
    get "/api/links"
    assert_response :unauthorized
  end

  test "index with a valid token returns only the user's links" do
    other = User.create!(email: "other@example.com", confirmed_at: Time.current)
    ts = (Time.now.to_f * 1000).to_i
    Link.create!(id: SecureRandom.uuid, url: "https://other.com",
                 read: 0, timestamp: ts, updated_at: ts, user: other)

    get "/api/links", headers: auth_headers(@user.api_token)
    assert_response :ok
    urls = JSON.parse(response.body).map { |l| l["url"] }
    assert_includes urls, "https://example.com"
    assert_not_includes urls, "https://other.com"
  end

  test "create associates the new link with the token's user" do
    assert_difference "Link.count", 1 do
      post "/api/links", params: { link: "https://created.com" }, headers: auth_headers(@user.api_token)
    end
    assert_response :created
    assert_equal @user, Link.find_by(url: "https://created.com").user
  end

  test "update only touches the user's own links" do
    other = User.create!(email: "x@example.com", confirmed_at: Time.current)
    ts = (Time.now.to_f * 1000).to_i
    foreign = Link.create!(id: SecureRandom.uuid, url: "https://foreign.com",
                           read: 0, timestamp: ts, updated_at: ts, user: other)

    patch "/api/links", params: { id: foreign.id, read: true }, headers: auth_headers(@user.api_token)
    assert_response :not_found
    assert_equal 0, foreign.reload.read
  end

  test "destroy only removes the user's own links" do
    delete "/api/links", params: { id: @link.id }, headers: auth_headers(@user.api_token)
    assert_response :ok
    assert_nil Link.find_by(id: @link.id)
  end
end
