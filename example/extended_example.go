package main

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/andreas-tiket/redis-memolock/memolock"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

// BindAddr contains the address for the HTTP Server to bind to.
const BindAddr = "127.0.0.1:8080"

func main() {
	memolock.InitLocalCache(nil)
	r := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // use default Addr
		Password: "",               // no password set
		DB:       0,                // use default DB
	})
	router := mux.NewRouter()
	srv := &http.Server{
		Handler: router,
		Addr:    BindAddr,
	}

	//  count:foo
	//  count/lock:foo
	//  count/notif:foo
	counter := 0
	countMemolock, _ := memolock.NewRedisMemoLock(r.Context(), r, "count", 10*time.Second)

	// GET counter
	router.HandleFunc("/counter", func(w http.ResponseWriter, r *http.Request) {

		requestTimeout := 10 * time.Second
		cachedQueryset, _, _ := countMemolock.GetResource(r.Context(), "counter", requestTimeout, func() (string, time.Duration, error) {
			fmt.Printf("(get count %d) Working hard!\n", counter)

			result := fmt.Sprintf("<query set result %d>", counter)
			// Simulate some hard work like fecthing data from a DBMS
			<-time.After(2 * time.Second)

			return result, 100 * time.Second, nil
		}, 0.8)

		fmt.Fprint(w, cachedQueryset)
	}).Methods("GET")

	// POST counter
	router.HandleFunc("/counter", func(w http.ResponseWriter, r *http.Request) {
		counter++
		countMemolock.InvalidateCache("counter")
		fmt.Fprint(w, "counter++")
	}).Methods("POST")

	//  query:foo
	//  query/lock:foo
	//  query/notif:foo
	queryMemolock, _ := memolock.NewRedisMemoLock(r.Context(), r, "query", 5*time.Second)

	// GET query/simple
	router.HandleFunc("/query/simple/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"] // extract {id} from the url path

		requestTimeout := 10 * time.Second
		cachedQueryset, _, _ := queryMemolock.GetResource(r.Context(), id, requestTimeout, func() (string, time.Duration, error) {
			fmt.Printf("(query/queryset/%s) Working hard!\n", id)

			// Simulate some hard work like fecthing data from a DBMS
			<-time.After(2 * time.Second)
			result := fmt.Sprintf("<query set result %s>", id)

			return result, 5 * time.Second, nil
		}, 0.8)

		fmt.Fprint(w, cachedQueryset)
	})

	//  report:foo
	//  report/lock:foo
	//  report/notif:foo
	reportMemolock, _ := memolock.NewRedisMemoLock(r.Context(), r, "report", 5*time.Second)

	// GET report/renewable
	router.HandleFunc("/report/renewable/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"] // extract {id} from the url path

		requestTimeout := 10 * time.Second
		reportPdfLocation, _ := reportMemolock.GetResourceRenewable(r.Context(), id, requestTimeout, func(renew memolock.LockRenewFunc) (string, time.Duration, error) {
			fmt.Printf("(report/renewable/%s) Working super-hard! (1)\n", id)
			<-time.After(2 * time.Second)

			// It turns out we have to do a lot of work, renew the lock!
			_ = renew(20 * time.Second)

			// Simulate some hard work
			<-time.After(6 * time.Second)
			fmt.Printf("(report/renewable/%s) Working super-hard! (2)\n", id)
			result := fmt.Sprintf("https://somewhere/%s-report.pdf", id)

			return result, 5 * time.Second, nil
		})

		fmt.Fprint(w, reportPdfLocation)
	})

	// GET report/oh-no
	router.HandleFunc("/report/oh-no/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"] // extract {id} from the url path

		requestTimeout := 10 * time.Second
		reportPdfLocation, err := reportMemolock.GetResourceRenewable(r.Context(), id, requestTimeout, func(renew memolock.LockRenewFunc) (string, time.Duration, error) {
			fmt.Printf("(report/oh-no/%s) Working super-hard! (1)\n", id)
			<-time.After(6 * time.Second)

			// It turns out we have to do a lot of work, renew the lock!
			// Oh no, we are already out of time!
			err := renew(20 * time.Second)
			if err != nil {
				return "", 0, err
			}

			// Simulate some hard work
			<-time.After(6 * time.Second)
			fmt.Printf("(report/renewable/%s) Working super-hard! (2)\n", id)
			result := fmt.Sprintf("https://somewhere/%s-report.pdf", id)

			return result, 50 * time.Second, nil
		})

		if err != nil {
			fmt.Fprintf(w, "ERROR: %v \n", err)
			return
		}

		fmt.Fprint(w, reportPdfLocation)
	})

	//  ext:foo
	//  ext/lock:foo
	//  ext/notif:foo
	extMemolock, err := memolock.NewRedisMemoLock(r.Context(), r, "ext", 15*time.Second)
	if err != nil {
		panic(err)
	}
	// GET ext/stemmer/forniture
	router.HandleFunc("/ext/stemmer/{word}", func(w http.ResponseWriter, r *http.Request) {
		word := mux.Vars(r)["word"] // extract {word} from the url path

		requestTimeout := 10 * time.Second
		stemming, _ := extMemolock.GetResourceExternal(r.Context(), word, requestTimeout, func() error {
			fmt.Printf("(ext/stemmer/%s) Working hard!\n", word)

			// We don't .Output() / try to read stdout, because we will be notified from Redis.
			go exec.Command("python3", "python_service/stemmer.py", word).Run()

			return nil
		})

		fmt.Fprint(w, stemming)
	})

	fmt.Println("Listening to ", BindAddr)
	fmt.Println("GET /query/simple/{foo}     -> simple caching")
	fmt.Println("GET /report/renewable/{foo} -> renewable lock ")
	fmt.Println("GET /report/oh-no/{bar}     -> failure case for renewable lock")
	fmt.Println("GET /ext/stemmer/{banana}   -> external program interop")

	srv.ListenAndServe()
}
