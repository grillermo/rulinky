# frozen_string_literal: true

require "test_helper"
require "webmock/minitest"

class ReaditsoonPreviewTest < ActiveSupport::TestCase
  PREVIEW_URL = "https://readitsoon.app/preview"

  def preview_html(title:, content:)
    <<~HTML
      <!DOCTYPE html>
      <html><body>
        <form action="/send" method="POST" x-data="previewSendForm()" x-init="init()">
          <input type="hidden" name="url" value="https://example.com">
          <input type="hidden" name="markdown_content" value="# md">
          <input type="hidden" name="title" value="#{title}">
          <input type="hidden" name="author" value="Jane">
        </form>
        <form action="/subscriptions/new" method="POST">
          <input type="hidden" name="html_content" value="#{content}">
        </form>
      </body></html>
    HTML
  end

  test "extracts title and html_content from preview response" do
    stub_request(:post, PREVIEW_URL)
      .to_return(status: 200, body: preview_html(title: "Clean Title", content: "<p>Body</p>"))

    result = ReaditsoonPreview.call("<html>raw</html>", "https://example.com")

    assert_equal "Clean Title", result[:title]
    assert_equal "<p>Body</p>", result[:content]
  end

  test "sends inputhtml, url and email in the request" do
    stub = stub_request(:post, PREVIEW_URL)
      .with(body: hash_including("inputhtml" => "<html>raw</html>", "url" => "https://example.com"))
      .to_return(status: 200, body: preview_html(title: "T", content: "C"))

    ReaditsoonPreview.call("<html>raw</html>", "https://example.com")

    assert_requested(stub)
  end

  test "raises on non-200 response" do
    stub_request(:post, PREVIEW_URL).to_return(status: 400, body: '{"error":"missing inputhtml"}')

    error = assert_raises(RuntimeError) do
      ReaditsoonPreview.call("<html>raw</html>", "https://example.com")
    end
    assert_match(/missing inputhtml/, error.message)
  end

  test "raises when previewSendForm is missing" do
    stub_request(:post, PREVIEW_URL)
      .to_return(status: 200, body: "<html><body><p>no form</p></body></html>")

    assert_raises(RuntimeError) do
      ReaditsoonPreview.call("<html>raw</html>", "https://example.com")
    end
  end
end
