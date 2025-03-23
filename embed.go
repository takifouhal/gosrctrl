package main

import _ "embed"

//go:embed generate_db.py
var generateDbScript []byte