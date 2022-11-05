package main

import "embed"

//go:embed views/*
var viewsDir embed.FS
