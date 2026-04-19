# frozen_string_literal: true

class AddContentToLinksAndCreateLinkContentJobs < ActiveRecord::Migration[8.1]
  def change
    add_column :links, :content, :text

    create_table :link_content_jobs, id: :string do |t|
      t.string :link_id, null: false
      t.string :status, null: false, default: "queued"
      t.text :error_message
      t.timestamps
    end

    add_index :link_content_jobs, :link_id
    add_index :link_content_jobs, :status
  end
end
