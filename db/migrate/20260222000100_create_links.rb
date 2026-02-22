# frozen_string_literal: true

class CreateLinks < ActiveRecord::Migration[8.1]
  def change
    create_table :links, id: :string do |t|
      t.text :url, null: false
      t.text :note
      t.integer :read, null: false, default: 0
      t.bigint :timestamp, null: false
      t.bigint :updated_at, null: false
    end

    add_index :links, :updated_at
    add_index :links, :read
  end
end

