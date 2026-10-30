# frozen_string_literal: true

require 'socket'

server = TCPServer.new('127.0.0.1', 3000)
loop do
  client = server.accept
  while (line = client.gets)
    break if line == "\r\n"
  end
  client.write "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 11\r\nConnection: close\r\n\r\nHello World"
  client.close
end
