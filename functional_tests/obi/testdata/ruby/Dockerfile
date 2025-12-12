FROM ruby:3.2-alpine

WORKDIR /app

# Copy application code
COPY app.rb .

# Install runtime dependency for Ruby 3.x
RUN gem install webrick --no-document

EXPOSE 8080

CMD ["ruby", "app.rb"]
