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

ActiveRecord::Schema[8.1].define(version: 2026_04_19_000000) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

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
    t.integer "read", default: 0, null: false
    t.bigint "timestamp", null: false
    t.bigint "updated_at", null: false
    t.text "url", null: false
    t.index ["id"], name: "index_links_on_id", unique: true
    t.index ["read"], name: "index_links_on_read"
    t.index ["updated_at"], name: "index_links_on_updated_at"
  end
end
