// SPDX-License-Identifier: BSD-3-Clause
// SPDX-FileCopyrightText: 2018-2025 Harald Sitter <sitter@kde.org>

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/activation"
	"github.com/folbricht/desync"
	"github.com/fsnotify/fsnotify"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// PrepareReaders can prepare subsequent readers in advance. This notably
// helps offset HTTP latency by issuing the requests ahead of time.
// We wrap all our LazyReaders in PrepareReaders and then activate them by
// doing empty reads on them. This spins up the HTTP request in advance with
// the hope that by the time we actually need to read from them, the request
// has received data already.
type PrepareReader struct {
	Reader io.Reader
	Next   []*PrepareReader
}

func (pr PrepareReader) Read(p []byte) (n int, err error) {
	if len(p) != 0 { // Only prepare if we aren't getting prepared ourself.
		for _, next := range pr.Next {
			next.Prepare()
		}
	}

	return pr.Reader.Read(p)
}

func (pr *PrepareReader) Prepare() error {
	_, err := pr.Read([]byte{}) // Empty read to trigger the LazyReader
	return err
}

func getTargetUrl(url *url.URL) (*url.URL, error) {
	resp, err := http.DefaultClient.Head(url.String())
	if err != nil {
		return nil, fmt.Errorf("Failed to get target URL: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Upstream for EROFS index responded with %d", resp.StatusCode)
	}
	targetURL := resp.Request.URL
	targetURL.Path = filepath.Dir(targetURL.Path) // Use the final redirected URL base path
	return targetURL, nil
}

type URLType int

const (
	UnknownURLType URLType = iota
	OriginURLType
	CDNURLType
	MirrorURLType
)

type HTTPContext struct {
	URLType URLType
	Cached  bool
}

func newHTTPContext(targetURL *url.URL) (HTTPContext, error) {
	upstream, err := url.Parse(globalConfig.UpstreamURL)
	if err != nil {
		return HTTPContext{}, fmt.Errorf(
			"invalid upstream URL: %w",
			err,
		)
	}

	if strings.EqualFold(
		targetURL.Hostname(),
		upstream.Hostname(),
	) {
		return HTTPContext{
			URLType: OriginURLType,
			Cached:  false,
		}, nil
	}

	resp, err := http.DefaultClient.Head(targetURL.String())
	if err != nil {
		return HTTPContext{URLType: UnknownURLType, Cached: false}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HTTPContext{URLType: UnknownURLType, Cached: false}, fmt.Errorf("Upstream for EROFS responded with %d", resp.StatusCode)
	}
	if resp.Header.Get("X-77-Cache") != "" {
		cached := strings.EqualFold(resp.Header.Get("X-77-Cache"), "HIT")
		return HTTPContext{URLType: CDNURLType, Cached: cached}, nil
	}
	return HTTPContext{URLType: MirrorURLType, Cached: true}, nil
}

func openStoreForHTTPContext(ctx HTTPContext, url *url.URL) (desync.Store, error) {
	if !globalConfig.EnableStore {
		log.Println("Store is disabled by configuration")
		return nil, nil
	}
	log.Println("Opening store for HTTP context:", ctx)

	makeStore := false
	// Things get even more complicated here. We want to have the assembler do as few requests as possible.
	// But that means we need to know if a request is likely to be slow or not. So... here we go
	switch ctx.URLType {
	case OriginURLType:
		// We are talking to a KDE server. They will have all the data but not necessarily at the speed we want.
		makeStore = true
	case CDNURLType:
		// We are talking to a CDN.
		if ctx.Cached {
			// If the data is cached here we can quickly access large chunks of data at good speeds. Perfect!
			// Nothing to do. We'll fall into the erofs seed and make range requests against it.
		} else {
			// If it is not cached we will want to make requests to the chunk store. We will need more requests but
			// the latency will be better as each request will need to round-trip to the origin.
			// If we requested against the erofs seed here we'd first have to wait for the CDN to stream the entire
			// file into cache.
			makeStore = true
		}
	case MirrorURLType:
		// We are talking to a mirror. This is in a way the best case. If we got here the mirror already has the entire
		// erofs cached and we can access large chunks of data at good speeds.
		// Nothing to do. We'll fall into the erofs seed and make range requests against it.
	case UnknownURLType:
		// We have no idea what we are talking to. Be pessimistic about it and use the chunk store.
		makeStore = true
	}

	if globalConfig.ForceStore {
		log.Println("Store usage is forced by configuration")
		makeStore = true
	}

	log.Println("HTTP URL type:", ctx.URLType)
	log.Println("Store usage decision:", makeStore)

	if makeStore {
		// TODO: should probably detect this somehow by asking a server where the store is.
		storeURL, err := url.Parse(globalConfig.StoreURL)
		if err != nil {
			desync.Log.Error("Failed to parse store URL:", err)
			panic(err)
		}
		store, err := desync.NewRemoteHTTPStore(storeURL, desync.NewStoreOptionsWithDefaults())
		if err != nil {
			desync.Log.Error("Failed to create remote HTTP store:", err)
			panic(err)
		}

		return store, nil
	}

	return nil, nil
}

func file(c *gin.Context) {
	fullpath := c.Request.URL.Path
	path := filepath.Dir(fullpath)
	file := filepath.Base(fullpath)

	url, err := url.Parse(globalConfig.UpstreamURL)
	if err != nil {
		panic(err)
	}
	url = url.JoinPath(strings.TrimPrefix(path, "/"))

	desync.Log.SetOutput(os.Stdout)

	desync.Log.Warn("Requested file:", fullpath)
	desync.Log.Warn("Using URL:", url.String())

	if filepath.Ext(file) != ".erofs" {
		c.Redirect(http.StatusTemporaryRedirect, url.JoinPath(file).String())
		return
	}

	// We make a bunch of HEAD requests to figure out what we are dealing with,
	// should be fast enough for this critical section of processing!
	url, err = getTargetUrl(url.JoinPath(file + ".caibx"))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to get target URL: %s", err)
		return
	}
	desync.Log.Warn("Redirecting to URL: ", url.String())

	httpContext, err := newHTTPContext(url.JoinPath(file))
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to get URL type: %s", err)
		return
	}

	remoteIndexStore, err := desync.NewRemoteHTTPIndexStore(url, desync.StoreOptions{})
	if err != nil {
		desync.Log.Warn("Failed to create remote index store: ", err)
		panic(err)
	}

	desync.Log.Warn("Using remote index store at", file)
	remoteIndex, err := remoteIndexStore.GetIndex(file + ".caibx")
	if err != nil {
		desync.Log.Warn("Failed to get index for", file, ":", err)
		panic(err)
	}

	erofses, err := filepath.Glob(globalConfig.SeedGlob)
	if err != nil {
		desync.Log.Warn("Failed to glob for erofses: ", err)
		panic(err)
	}

	erofsStore, err := desync.NewLocalIndexStore(globalConfig.LocalIndexStorePath)
	if err != nil {
		panic(err)
	}
	defer erofsStore.Close()

	seeds := []desync.Seed{}
	for _, erofsPath := range erofses {
		erofs := filepath.Base(erofsPath)
		desync.Log.Warn("Using erofs as seed", erofs)

		index, err := erofsStore.GetIndex(erofs + ".caibx")
		if err != nil {
			desync.Log.Warnln("Failed to get index for", erofs, ":", err)
			continue
		}

		seed, err := NewIndexSeed("", erofsPath, index)
		if err != nil {
			desync.Log.Warn("Failed to create seed for", erofs, ":", err)
			continue
		}
		seeds = append(seeds, seed)
	}

	store, err := openStoreForHTTPContext(httpContext, url.JoinPath(file))
	if err != nil {
		desync.Log.Error("Failed to open stores for HTTP context:", err)
		panic(err)
	}
	defer func() {
		if store == nil {
			return
		}

		if err := store.Close(); err != nil {
			desync.Log.Error("Failed to close store:", err)
		}
	}()

	assembler, err := stream(c, remoteIndex, NewHTTPSeed(url.JoinPath(file), remoteIndex), store, seeds, AssembleOptions{})
	if err != nil {
		desync.Log.Error("Failed to create stream:", err)
		panic(err)
	}
	// NOTE: this returns LazyReaders! The lazy readers only open the internal reader when Read() is called!
	readClosers := assembler.Readers()
	defer func() {
		for _, rc := range readClosers {
			rc.Close()
		}
	}()

	desync.Log.Debugln("Read closers created")

	prepareReaders := make([]*PrepareReader, len(readClosers))
	for i, rc := range readClosers {
		prepareReaders[i] = &PrepareReader{Reader: rc, Next: []*PrepareReader{}} // we'll fill Next later
	}

	for i, pr := range prepareReaders {
		preparationCount := 2
		if store != nil {
			preparationCount = 90
		}
		for j := 0; j < preparationCount; j++ {
			next := prepareReaders[(i+j)%len(prepareReaders)]
			pr.Next = append(pr.Next, next)
		}
	}

	readers := make([]io.Reader, len(prepareReaders))
	for i, pr := range prepareReaders {
		readers[i] = pr
	}

	c.DataFromReader(http.StatusOK, remoteIndex.Length(), "application/octet-stream",
		io.MultiReader(readers...), map[string]string{})

	// Mind that gin will print c.Errors, so we don't need to print them manually to see why
	// a transfer broke.
}

func updateSize(ctx *gin.Context) {
	version := ctx.Query("version")
	if version == "" {
		ctx.String(http.StatusBadRequest, "version parameter not set")
		return
	}

	updateSizer := NewUpdateSizer(version)
	context := updateSizer.PrepareContext()

	size, err := globalCache.GetUpdateSize(context)
	if err == os.ErrNotExist { // no cache hit
		size, err = updateSizer.Calculate(ctx)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to calculate size: %s", err)
			return
		}

		globalCache.SetUpdateSize(size, context)
	} else if err != nil {
		ctx.String(http.StatusInternalServerError, "Failed to update size: %s", err)
		return
	}

	ctx.String(http.StatusOK, strconv.FormatUint(size, 10))
}

var globalCache Cache
var globalConfig Config

func main() {
	flag.Parse()

	err := sentry.Init(sentry.ClientOptions{
		Dsn: "",
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	globalCache = LoadCache("/run/kde-linux-sysupdated/globalCache.json")
	defer globalCache.sync()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Println("event:", event)
				if event.Has(fsnotify.Write) && event.Name == DefaultConfigPath {
					log.Println("Reloading config due to write event")
					// Technically subject to a thread race but in practice the config isn't meant to change much at all.
					globalConfig = LoadConfig(DefaultConfigPath)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("fsnotify error:", err)
			}
		}
	}()

	globalConfig = LoadConfig(DefaultConfigPath)
	err = watcher.Add(DefaultConfigPath)
	if err != nil {
		log.Println("Failed to watch config file:", err)
	}

	log.Println("Ready to rumble...")
	router := gin.Default(func(e *gin.Engine) {
		e.ContextWithFallback = true
	})
	router.Use(activityTracker())

	router.GET("/v1/updatesize", updateSize)
	router.NoRoute(file)

	listeners, err := activation.Listeners()
	if err != nil {
		panic(err)
	}

	log.Println("starting servers")
	var servers []*http.Server
	for _, listener := range listeners {
		server := &http.Server{Handler: router}
		go server.Serve(listener)
		servers = append(servers, server)
	}

	if len(servers) == 0 {
		log.Println("servers empty. adding manual server")
		panic("no listeners found, please run with systemd socket activation or use `make run`")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		<-idleTimer.timer.C
		quit <- syscall.SIGQUIT
	}()

	// Wait for some quit cause.
	// This could be INT, TERM, QUIT or the db update trigger.
	// We'll then do a zero downtime shutdown.
	// This relies on systemd managing the socket and us doing graceful listener
	// shutdown. Once we are no longer listening, the system starts backlogging
	// the socket until we get restarted and listen again.
	// Ideally this results in zero dropped connections.
	<-quit
	log.Println("servers are shutting down")

	for _, srv := range servers {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Server Shutdown: %s", err)
		}
	}

	log.Println("Server exiting")
}
