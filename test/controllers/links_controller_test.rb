# frozen_string_literal: true

require "test_helper"

class LinksControllerTest < ActionDispatch::IntegrationTest
  def setup
    # Create a test link directly — no fixtures needed
    @link = Link.create!(
      id: "test-link-id-001",
      url: "https://example.com",
      note: "Test note",
      read: 0,
      timestamp: (Time.now.to_f * 1000).to_i,
      updated_at: (Time.now.to_f * 1000).to_i
    )
  end

  def teardown
    Link.delete_all
  end

  test "index renders Inertia component with expected props" do
    get root_path, headers: { "X-Inertia" => "true", "X-Inertia-Version" => "1" }
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
