# frozen_string_literal: true

class SeedOwnerAndBackfillLinks < ActiveRecord::Migration[8.1]
  def up
    User.reset_column_information
    owner = User.find_or_create_by!(email: "hola@grillermo.com") do |u|
      u.confirmed_at = Time.current
    end

    execute <<~SQL.squish
      UPDATE links SET user_id = #{owner.id} WHERE user_id IS NULL
    SQL

    change_column_null :links, :user_id, false
  end

  def down
    change_column_null :links, :user_id, true
  end
end
