package main

// selfUpdateProduct identifies which launcher this module builds. The shared
// self-update engine (selfupdate.go, byte-identical in both modules) uses it
// to select the matching artifacts in the release manifest.
const selfUpdateProduct = "viewer"
