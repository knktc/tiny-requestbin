Tiny RequestBin
================

Tiny RequestBin is a lightweight HTTP request inspector written in Go.

Quick start
-----------

1. Extract this archive.
2. Open a terminal in the extracted directory.
3. Run the binary:

   macOS / Linux:
     ./tiny-requestbin

   Windows:
     tiny-requestbin.exe

4. Open the web UI:

   http://127.0.0.1:8282/

HTTPS example
-------------

If you already have a local certificate and key, you can start with HTTPS:

  tiny-requestbin -port 9443 -listen 127.0.0.1 -tls-cert ./localhost.pem -tls-key ./localhost-key.pem

CLI mode
--------

To also print captured requests in the terminal:

  tiny-requestbin -cli

Useful flags
------------

  -port       Port to listen on. Default: 8282
  -listen     Address to listen on. Default: 127.0.0.1
  -max        Maximum number of requests to store. Default: 100
  -cli        Print captured requests to the terminal
  -tls-cert   TLS certificate file
  -tls-key    TLS private key file

macOS note
----------

If macOS blocks the downloaded binary on first launch, run:

  chmod +x tiny-requestbin
  xattr -d com.apple.quarantine tiny-requestbin

Project page
------------

GitHub:
  https://github.com/knktc/tiny-requestbin
