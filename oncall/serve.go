// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"net/http"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/x/devtools/internal/cache"
	"v.io/x/lib/cmdline"
)

var (
	addressFlag   string
	cacheFlag     string
	staticDirFlag string
)

func init() {
	cmdServe.Flags.StringVar(&addressFlag, "address", ":8000", "Listening address for the server.")
	cmdServe.Flags.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	cmdServe.Flags.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
}

// cmdServe represents the 'serve' command of the oncall tool.
var cmdServe = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runServe),
	Name:   "serve",
	Short:  "Serve oncall dashboard data from Google Storage",
	Long:   "Serve oncall dashboard data from Google Storage.",
}

func runServe(env *cmdline.Env, _ []string) (e error) {
	jirix, err := jiri.NewX(env)
	if err != nil {
		return err
	}

	// Set up the root/cache directory.
	root := cacheFlag
	if root == "" {
		tmpDir, err := jirix.NewSeq().TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
		root = tmpDir
	}

	// Start server.
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		dataHandler(jirix, root, w, r)
	})
	http.HandleFunc("/pic", func(w http.ResponseWriter, r *http.Request) {
		picHandler(jirix, root, w, r)
	})
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/", staticHandler)
	if err := http.ListenAndServe(addressFlag, nil); err != nil {
		return fmt.Errorf("ListenAndServe(%s) failed: %v", addressFlag, err)
	}

	return nil
}

func dataHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	// Get timestamp from either the "latest" file or "ts" parameter.
	r.ParseForm()
	ts := r.Form.Get("ts")
	if ts == "" {
		var err error
		ts, err = readGoogleStorageFile(jirix, "latest")
		if err != nil {
			respondWithError(jirix, err, w)
			return
		}
	}

	cachedFile, err := cache.StoreGoogleStorageFile(jirix, root, bucketData, ts+".oncall")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	bytes, err := jirix.NewSeq().ReadFile(cachedFile)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(bytes)
}

func picHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	// Parameter "id" specifies the id of the pic.
	id := r.Form.Get("id")
	if id == "" {
		respondWithError(jirix, fmt.Errorf("parameter 'id' not found"), w)
		return
	}
	// Read picture file from Google Storage.
	cachedFile, err := cache.StoreGoogleStorageFile(jirix, root, bucketPics, id+".png")
	if err != nil {
		// Read "_unknown.jpg" as fallback.
		cachedFile, err = cache.StoreGoogleStorageFile(jirix, root, bucketPics, "_unknown.jpg")
		if err != nil {
			respondWithError(jirix, err, w)
			return
		}
	}
	bytes, err := jirix.NewSeq().ReadFile(cachedFile)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-control", "public, max-age=2592000")
	w.Write(bytes)
}

func respondWithError(jirix *jiri.X, err error, w http.ResponseWriter) {
	fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func readGoogleStorageFile(jirix *jiri.X, filename string) (string, error) {
	var out bytes.Buffer
	if err := jirix.NewSeq().Capture(&out, &out).Last("gsutil", "-q", "cat", bucketData+"/"+filename); err != nil {
		return "", err
	}
	return out.String(), nil
}
