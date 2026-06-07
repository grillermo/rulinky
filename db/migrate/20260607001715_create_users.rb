# frozen_string_literal: true

class CreateUsers < ActiveRecord::Migration[8.1]
  def change
    create_table :users do |t|
      t.string :email, null: false, default: ""

      # OTP
      t.string   :otp_code
      t.datetime :otp_expires_at

      # confirmation (null = unconfirmed)
      t.datetime :confirmed_at

      # API auth
      t.string :api_token

      # Devise :rememberable
      t.datetime :remember_created_at

      # Devise :trackable
      t.integer  :sign_in_count, default: 0, null: false
      t.datetime :current_sign_in_at
      t.datetime :last_sign_in_at
      t.string   :current_sign_in_ip
      t.string   :last_sign_in_ip

      t.timestamps
    end

    add_index :users, :email, unique: true
    add_index :users, :api_token, unique: true
  end
end
