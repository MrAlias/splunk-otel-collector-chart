require 'webrick'
require 'json'
require 'net/http'
require 'uri'

port = ENV['SERVER_PORT'] || '8080'
server = WEBrick::HTTPServer.new(:Port => port.to_i, :AccessLog => [], :Logger => WEBrick::Log.new($stderr, WEBrick::Log::INFO))

server.mount_proc('/health') do |req, res|
  res.content_type = 'application/json'
  res.status = 200
  res.body = JSON.generate({ status: 'ok' })
end

server.mount_proc('/chain') do |req, res|
  begin
    res.content_type = 'application/json'

    if req.request_method != 'POST'
      res.status = 405
      res.body = JSON.generate({
        service: 'ruby',
        status: 405,
        error: 'Only POST requests are allowed'
      })
      next
    end

    # Parse request body
    body = req.body
    data = JSON.parse(body)
    targets = data['targets'] || []

    $stderr.puts "Received chain request with #{targets.length} targets"

    # If no targets, return success (end of chain)
    if targets.empty?
      res.status = 200
      res.body = JSON.generate({
        service: 'ruby',
        status: 200,
        targets: targets,
        result: 'Chain completed'
      })
      next
    end

    # Forward to next target with remaining targets
    next_target = targets[0]
    remaining_targets = targets[1..-1]

    $stderr.puts "Forwarding to #{next_target} with #{remaining_targets.length} remaining targets"

    begin
      next_req = { targets: remaining_targets }
      chain_url = "http://#{next_target}/chain"
      uri = URI(chain_url)

      http = Net::HTTP.new(uri.host, uri.port)
      http.read_timeout = 10
      http.open_timeout = 10

      headers = { 'Content-Type' => 'application/json' }
      # Forward trace headers
      trace_headers = [
        'traceparent', 'tracestate',
        'b3', 'x-b3-traceid', 'x-b3-spanid', 'x-b3-sampled',
        'x-ot-span-context'
      ]
      trace_headers.each do |h|
        v = req[h]
        headers[h] = v if v && !v.empty?
      end

      request = Net::HTTP::Post.new(uri.path, headers)
      request.body = JSON.generate(next_req)

      response = http.request(request)
      res.status = response.code.to_i
      res.body = response.body

    rescue => error
      res.status = 502
      res.body = JSON.generate({
        service: 'ruby',
        status: 502,
        targets: targets,
        error: "Failed to call #{next_target}: #{error.message}"
      })
    end

  rescue => error
    res.content_type = 'application/json'
    res.status = 400
    res.body = JSON.generate({
      service: 'ruby',
      status: 400,
      error: "Error processing request: #{error.message}"
    })
  end
end

trap('INT') { server.shutdown }

$stderr.puts "Starting server on port #{port}"
server.start
