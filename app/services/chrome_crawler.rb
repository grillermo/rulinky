# frozen_string_literal: true

require "selenium-webdriver"
require "fileutils"

# Drives the user's real Chrome profile to fetch a fully-rendered, logged-in
# page, then cleans the DOM the same way the readitsoon bookmarklet does before
# returning the HTML.
#
# Chrome blocks DevTools on its default user-data-dir, so we copy the profile's
# cookies to a dedicated automation dir before each run. The automation dir is
# NOT the default Chrome dir, which lets Chrome expose the DevTools port.
class ChromeCrawler
  REAL_USER_DATA_DIR = ENV.fetch(
    "CHROME_REAL_USER_DATA_DIR",
    File.expand_path("~/Library/Application Support/Google/Chrome"),
  )
  AUTOMATION_USER_DATA_DIR = ENV.fetch(
    "CHROME_USER_DATA_DIR",
    File.expand_path("~/Library/Application Support/Google/ChromeRulinky"),
  )
  PROFILE_DIRECTORY = ENV.fetch("CHROME_PROFILE_DIRECTORY", "Default")
  PAGE_LOAD_TIMEOUT = 60

  # Port of readitsoon's bookmarklet cleaning (lib/bookmarklet.js lines 52-70).
  CLEAN_JS = <<~JS
    var isTwitter = /^https?:\\/\\/(www\\.)?(x\\.com|twitter\\.com)\\//.test(window.location.href);
    var articleEl = isTwitter ? document.querySelector('[data-testid="twitterArticleReadView"]') : null;
    var clone;
    if (articleEl) {
      clone = document.createElement("html");
      clone.appendChild(document.createElement("body"));
      clone.querySelector("body").appendChild(articleEl.cloneNode(true));
    } else {
      clone = document.documentElement.cloneNode(true);
    }
    var junk = clone.querySelectorAll("script,style,canvas,select,textarea");
    for (var i = junk.length - 1; i >= 0; i--) {
      if (junk[i].parentNode) junk[i].parentNode.removeChild(junk[i]);
    }
    var titleH1 = document.createElement("h1");
    titleH1.textContent = document.title;
    var cloneBody = clone.querySelector("body") || clone;
    cloneBody.insertBefore(titleH1, cloneBody.firstChild);
    return "<!DOCTYPE html>\\n" + clone.outerHTML;
  JS

  def self.call(url)
    new(url).call
  end

  def initialize(url)
    @url = url
  end

  def call
    Rails.logger.info("[ChromeCrawler] start url=#{@url}")
    sync_profile
    kill_chrome
    Rails.logger.info("[ChromeCrawler] building driver profile=#{PROFILE_DIRECTORY}")
    driver = build_driver
    driver.manage.timeouts.page_load = PAGE_LOAD_TIMEOUT
    Rails.logger.info("[ChromeCrawler] navigating to url=#{@url} timeout=#{PAGE_LOAD_TIMEOUT}s")
    driver.get(@url)
    Rails.logger.info("[ChromeCrawler] page loaded, running CLEAN_JS")
    html = driver.execute_script(CLEAN_JS)
    Rails.logger.info("[ChromeCrawler] done html_bytes=#{html&.bytesize}")
    html
  ensure
    Rails.logger.info("[ChromeCrawler] quitting driver")
    driver&.quit
  end

  private

  # Copy session-bearing files from the real Chrome profile to the automation
  # dir. Chrome refuses DevTools on its default profile dir, so we use this
  # copy as the --user-data-dir instead.
  def sync_profile
    automation_profile = File.join(AUTOMATION_USER_DATA_DIR, PROFILE_DIRECTORY)
    FileUtils.mkdir_p(automation_profile)

    real_profile = File.join(REAL_USER_DATA_DIR, PROFILE_DIRECTORY)
    files_to_sync = %w[Cookies "Local Storage" "Session Storage" "IndexedDB"]

    files_to_sync.each do |name|
      src = File.join(real_profile, name)
      dst = File.join(automation_profile, name)
      next unless File.exist?(src)

      FileUtils.cp_r(src, dst)
      Rails.logger.info("[ChromeCrawler] synced #{name} to automation profile")
    end
  end

  def kill_chrome
    Rails.logger.info("[ChromeCrawler] killing Chrome")
    system("pkill", "-i", "Google Chrome")
    deadline = Time.now + 15
    until Time.now > deadline
      still_running = system("pgrep", "-i", "Google Chrome", out: File::NULL, err: File::NULL)
      break unless still_running
      Rails.logger.info("[ChromeCrawler] waiting for Chrome to exit...")
      sleep 0.5
    end
    Rails.logger.info("[ChromeCrawler] Chrome exited, proceeding")
    sleep 0.5
  end

  def build_driver
    options = Selenium::WebDriver::Chrome::Options.new
    options.add_argument("--user-data-dir=#{AUTOMATION_USER_DATA_DIR}")
    options.add_argument("--profile-directory=#{PROFILE_DIRECTORY}")
    options.add_argument("--no-first-run")
    options.add_argument("--no-default-browser-check")
    options.add_argument("--no-restore-last-session")
    options.add_argument("--disable-session-crashed-bubble")
    options.add_argument("--disable-sync")

    client = Selenium::WebDriver::Remote::Http::Default.new
    client.open_timeout = 120
    client.read_timeout = 120

    service = Selenium::WebDriver::Chrome::Service.new(
      log: "/tmp/chromedriver.log",
      args: ["--verbose", "--log-path=/tmp/chromedriver.log"]
    )

    Rails.logger.info("[ChromeCrawler] launching Selenium automation_dir=#{AUTOMATION_USER_DATA_DIR}")
    Selenium::WebDriver.for(:chrome, options: options, http_client: client, service: service)
  end
end
