# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.1].define(version: 2026_06_07_002647) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

  create_table "informant_error_groups", force: :cascade do |t|
    t.string "controller_action"
    t.datetime "created_at", null: false
    t.bigint "duplicate_of_id"
    t.string "error_class", null: false
    t.string "fingerprint", null: false
    t.string "first_backtrace_line"
    t.datetime "first_seen_at", null: false
    t.datetime "fix_deployed_at"
    t.string "fix_pr_url"
    t.string "fix_sha"
    t.string "job_class"
    t.datetime "last_notified_at"
    t.datetime "last_occurrence_stored_at"
    t.datetime "last_seen_at", null: false
    t.text "message"
    t.text "notes"
    t.string "original_sha"
    t.datetime "resolved_at"
    t.string "severity", default: "error"
    t.string "status", default: "unresolved", null: false
    t.integer "total_occurrences", default: 0, null: false
    t.datetime "updated_at", null: false
    t.index ["duplicate_of_id"], name: "index_informant_error_groups_on_duplicate_of_id"
    t.index ["error_class"], name: "index_informant_error_groups_on_error_class"
    t.index ["fingerprint"], name: "index_informant_error_groups_on_fingerprint", unique: true
    t.index ["status", "last_seen_at"], name: "index_informant_error_groups_on_status_and_last_seen_at"
    t.index ["status", "original_sha"], name: "index_informant_error_groups_on_status_and_original_sha"
    t.index ["status", "resolved_at"], name: "index_informant_error_groups_on_status_and_resolved_at"
    t.index ["status", "total_occurrences"], name: "index_informant_error_groups_on_status_and_total_occurrences"
    t.index ["status", "updated_at"], name: "index_informant_error_groups_on_status_and_updated_at"
    t.check_constraint "duplicate_of_id IS NULL OR duplicate_of_id <> id", name: "check_no_self_duplicate"
  end

  create_table "informant_occurrences", force: :cascade do |t|
    t.jsonb "backtrace"
    t.jsonb "breadcrumbs"
    t.datetime "created_at", null: false
    t.jsonb "custom_context"
    t.jsonb "environment_context"
    t.bigint "error_group_id", null: false
    t.jsonb "exception_chain"
    t.string "git_sha"
    t.jsonb "request_context"
    t.datetime "updated_at", null: false
    t.jsonb "user_context"
    t.index ["created_at"], name: "index_informant_occurrences_on_created_at"
    t.index ["error_group_id", "created_at"], name: "index_informant_occurrences_on_error_group_id_and_created_at"
    t.index ["error_group_id"], name: "index_informant_occurrences_on_error_group_id"
  end

  create_table "link_content_jobs", id: :string, force: :cascade do |t|
    t.datetime "created_at", null: false
    t.text "error_message"
    t.string "link_id", null: false
    t.string "status", default: "queued", null: false
    t.datetime "updated_at", null: false
    t.index ["link_id"], name: "index_link_content_jobs_on_link_id"
    t.index ["status"], name: "index_link_content_jobs_on_status"
  end

  create_table "links", id: :string, force: :cascade do |t|
    t.text "content"
    t.text "note"
    t.text "raw_title"
    t.integer "read", default: 0, null: false
    t.bigint "timestamp", null: false
    t.bigint "updated_at", null: false
    t.text "url", null: false
    t.bigint "user_id", null: false
    t.index ["id"], name: "index_links_on_id", unique: true
    t.index ["read"], name: "index_links_on_read"
    t.index ["updated_at"], name: "index_links_on_updated_at"
    t.index ["user_id"], name: "index_links_on_user_id"
  end

  create_table "users", force: :cascade do |t|
    t.string "api_token"
    t.datetime "confirmed_at"
    t.datetime "created_at", null: false
    t.datetime "current_sign_in_at"
    t.string "current_sign_in_ip"
    t.string "email", default: "", null: false
    t.datetime "last_sign_in_at"
    t.string "last_sign_in_ip"
    t.string "otp_code"
    t.datetime "otp_expires_at"
    t.datetime "remember_created_at"
    t.integer "sign_in_count", default: 0, null: false
    t.datetime "updated_at", null: false
    t.index ["api_token"], name: "index_users_on_api_token", unique: true
    t.index ["email"], name: "index_users_on_email", unique: true
  end

  add_foreign_key "informant_error_groups", "informant_error_groups", column: "duplicate_of_id"
  add_foreign_key "informant_occurrences", "informant_error_groups", column: "error_group_id"
  add_foreign_key "links", "users"
end
