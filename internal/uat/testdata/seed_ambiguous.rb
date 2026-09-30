# run: bundle exec rails runner tmp/uat_seed.rb
abort('production') if Rails.env.production?
PACKAGE = Package.joins(:app).merge(App.is_qontak_apps)
  .where(active: true)
  .order(:id).first
other = Package.joins(:app).where('name LIKE ?', '%qontak%').first
client = Api.new(password: 'hunter2x')
Client::Invoiceable.where(company_id: 1).delete_all
