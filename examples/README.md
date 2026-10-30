# Hello World examples

Each example serves `Hello World` through lserve.

## PHP

```bash
cd examples/php
lserve up
```

Open https://php.localhost:8443. No application server is needed.

## Node.js

```bash
cd examples/node
npm start
```

In another terminal, run `lserve up` from the same directory. Open https://hello-node.localhost:8443.

## Ruby

```bash
cd examples/ruby
ruby server.rb
```

In another terminal, run `lserve up` from the same directory. Open https://hello-ruby.localhost:8443.

## Python

```bash
cd examples/python
python3 server.py
```

In another terminal, run `lserve up` from the same directory. Open https://hello-python.localhost:8443.

## Go

```bash
cd examples/go
go run .
```

In another terminal, run `lserve up` from the same directory. Open https://hello-go.localhost:8443.

Stop serving an example with `lserve down` from its directory. Stop its application server with Ctrl+C.
