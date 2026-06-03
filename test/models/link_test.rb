# frozen_string_literal: true

require "test_helper"

class LinkTest < ActiveSupport::TestCase
  def build_link(attrs = {})
    Link.new({
      id: SecureRandom.uuid,
      url: "https://example.com",
      timestamp: (Time.now.to_f * 1000).to_i,
      updated_at: (Time.now.to_f * 1000).to_i,
    }.merge(attrs))
  end

  test "content_title returns raw_title when present" do
    link = build_link(raw_title: "My Article", content: "<title>Ignored</title>")
    assert_equal "My Article", link.content_title
  end

  test "content_title falls back to content title when raw_title blank" do
    link = build_link(raw_title: "", content: "<html><head><title>From Content</title></head></html>")
    assert_equal "From Content", link.content_title
  end

  test "content_title is nil when raw_title and content both blank" do
    link = build_link(raw_title: nil, content: nil)
    assert_nil link.content_title
  end
end
