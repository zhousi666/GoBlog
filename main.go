package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

const DEBUG bool = false
const STATIC_FILES_DIR string = "/root/GoBlog"

type BlogDetails struct {
	Blogname string `json:"blogname"`
	Website  string `json:"website"`
}

func init() {
	// Handles db/bucket creation
	db, err := bolt.Open("goblog.db", 0600, nil)

	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("UsersBucket")) // email -> password
		if err != nil {
			return fmt.Errorf("Error with UsersBucket: %s", err)
		}
		return nil
	})
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("CookieBucket")) // random string -> email
		if err != nil {
			return fmt.Errorf("Error with CookieBucket: %s", err)
		}
		return nil
	})
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("BlogMappingBucket")) // random string -> email
		if err != nil {
			return fmt.Errorf("Error with BlogMappingBucket: %s", err)
		}
		return nil
	})
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("UserToBlog")) // user -> blogdetails
		if err != nil {
			return fmt.Errorf("Error with UserToBlog: %s", err)
		}
		return nil
	})
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("PortBucket")) // port -> blog
		if err != nil {
			return fmt.Errorf("Error with PortBucket: %s", err)
		}
		return nil
	})
}

func LoginPage(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	username := getUser(w, req)
	if username == "" {
		baseT := template.Must(template.New("base").Parse(base))
		baseT = template.Must(baseT.Parse(login))

		baseT.ExecuteTemplate(w, "base", map[string]string{
			"PageName": "login",
			"User":     getUser(w, req),
		})
	} else {
		http.Redirect(w, req, "/admin/", http.StatusFound)
	}
}

func LoginHandler(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	email := req.FormValue("email")
	password := req.FormValue("password")

	if verifyUser(w, req, email, password) {
		http.Redirect(w, req, "/admin/", http.StatusFound)
	} else {
		http.Redirect(w, req, "/error/Invalid email or password", http.StatusFound)
	}
}

func LogoutHandler(w http.ResponseWriter, req *http.Request, p httprouter.Params) {
	cookie, err := req.Cookie("goblog")
	if err != nil {
		fmt.Println(err)
	}
	delete := http.Cookie{Name: "goblog", Value: "delete", Expires: time.Now(), HttpOnly: true, Path: "/"}
	http.SetCookie(w, &delete)
	db, err := bolt.Open("goblog.db", 0600, nil)
	if err != nil {
		fmt.Println(err)
	}
	defer db.Close()
	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("CookieBucket"))
		err := b.Delete([]byte(cookie.Value))
		return err
	})
	http.Redirect(w, req, "/", http.StatusFound)
}

func MainPage(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	username := getUser(w, r)
	if username == "" {
		baseT := template.Must(template.New("base").Parse(newMainPage))

		baseT.ExecuteTemplate(w, "base", map[string]string{
			"PageName": "main",
			"User":     getUser(w, r),
		})
	} else {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	}
}

func ErrorPage(w http.ResponseWriter, r *http.Request, pm httprouter.Params) {
	baseT := template.Must(template.New("base").Parse(base))
	baseT = template.Must(baseT.Parse(errorPage))

	baseT.ExecuteTemplate(w, "base", map[string]string{
		"PageName": "error",
		"User":     getUser(w, r),
		"Error":    pm.ByName("errorcode"),
	})
}

func SignupPage(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	baseT := template.Must(template.New("base").Parse(base))
	baseT = template.Must(baseT.Parse(signup))

	baseT.ExecuteTemplate(w, "base", map[string]string{
		"PageName": "signup",
		"User":     getUser(w, r),
	})
}

func SignupHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	if addUser(email, password) {
		cookie := http.Cookie{Name: "goblog", Value: RandomString(), Expires: time.Now().Add(time.Hour * 24 * 7 * 52), HttpOnly: true, MaxAge: 50000, Path: "/"}
		http.SetCookie(w, &cookie)
		db, err := bolt.Open("goblog.db", 0600, nil)
		if err != nil {
			fmt.Println(err)
		}
		defer db.Close()
		if err != nil {
			fmt.Println(err)
		}
		db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("CookieBucket"))
			err = b.Put([]byte(cookie.Value), []byte(email))
			return err
		})
		if err != nil {
			fmt.Println(err)
		}
		http.Redirect(w, r, "/admin/", http.StatusFound)
	} else {
		fmt.Println("Failure!")
		http.Redirect(w, r, "/signup/", http.StatusFound)
	}
}

func AdminPage(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	success := r.FormValue("success")

	username := getUser(w, r)
	if username != "" {
		db, err := bolt.Open("goblog.db", 0600, nil)
		if err != nil {
			fmt.Println(err)
		}
		defer db.Close()

		baseT := template.Must(template.New("base").Parse(base))
		baseT = template.Must(baseT.Parse(admin))

		baseT.ExecuteTemplate(w, "base", map[string]interface{}{
			"PageName": "admin",
			"User":     username,
			"Blogs":    getBlogsForUser(db, username),
			"Success":  success,
		})
	} else {
		http.Redirect(w, r, "/error/You must be authenticated!", http.StatusFound)
	}
}

func BlogCreationHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	blogname := r.FormValue("blogname")
	//	db, err := bolt.Open("goblog.db", 0600, nil)
	//	if err != nil {
	//		fmt.Println(err)
	//	}
	//	defer db.Close()
	//	db.Update(func(tx *bolt.Tx) error {
	//		b := tx.Bucket([]byte("PortBucket"))
	//		port, _ = b.NextSequence()
	//		return err
	//	})
	//	if err != nil {
	//		fmt.Println(err)
	//	}

	// If for some reason autoincrementing doesn't work, resort to traditional method.
	seed := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(seed)
	port := rng.Intn(63000) + 2000

	/*
		website, err := checkUrl(websiteOriginal)
			if err != nil {
				http.Redirect(w, r, fmt.Sprintf("/error/%s is not a valid url", websiteOriginal), http.StatusFound)
				return
			}
	*/

	re := regexp.MustCompile("[^A-Za-z]")
	blogname = re.ReplaceAllString(blogname, "")
	website := blogname + ".goblog.pw"

	blogcheck := []byte("")

	username := getUser(w, r)
	if username != "" {
		db, err := bolt.Open("goblog.db", 0600, nil)
		if err != nil {
			fmt.Println(err)
		}
		defer db.Close()
		db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("BlogMappingBucket"))
			blogcheck = b.Get([]byte(blogname))
			return nil
		})

		if blogcheck == nil && len(blogname) > 1 {
			create, err := exec.Command("./create.sh", blogname, website, strconv.Itoa(port)).Output()
			if err != nil && !DEBUG {
				fmt.Println(err)
			} else {
				fmt.Println("80 -> " + strconv.Itoa(port))
				fmt.Println(string(create))
				db.Update(func(tx *bolt.Tx) error {
					b := tx.Bucket([]byte("BlogMappingBucket"))
					err := b.Put([]byte(blogname), []byte(website))
					return err
				})
				addBlogToUser(db, username, blogname, website)
				http.Redirect(w, r, "/admin/", http.StatusFound)
				return
			}
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	} else {
		http.Redirect(w, r, "/error/You must be authenticated!", http.StatusFound)
		return
	}
}

func addBlogToUser(db *bolt.DB, username string, blogname string, website string) {
	existingblogs := []byte("")

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("UserToBlog"))
		existingblogs = b.Get([]byte(username))
		return nil
	})

	var v []BlogDetails = make([]BlogDetails, 0)
	json.Unmarshal(existingblogs, &v)

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("UserToBlog"))
		v = append(v, BlogDetails{blogname, website})
		m, _ := json.Marshal(v)
		err := b.Put([]byte(username), m)
		return err
	})
}

func getBlogsForUser(db *bolt.DB, username string) []BlogDetails {
	existingblogs := []byte("")

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("UserToBlog"))
		existingblogs = b.Get([]byte(username))
		return nil
	})

	var v []BlogDetails = make([]BlogDetails, 0)
	json.Unmarshal(existingblogs, &v)

	return v
}

func verifyUser(w http.ResponseWriter, r *http.Request, email string, password string) bool {
	correctpass := []byte("")
	inputpass := []byte(password)
	db, err := bolt.Open("goblog.db", 0600, nil)
	if err != nil {
		fmt.Println(err)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("UsersBucket"))
		correctpass = b.Get([]byte(email))
		return nil
	})
	if bcrypt.CompareHashAndPassword(correctpass, inputpass) == nil {
		cookie := http.Cookie{Name: "goblog", Value: RandomString(), Expires: time.Now().Add(time.Hour * 24 * 7 * 52), HttpOnly: true, MaxAge: 50000, Path: "/"}
		http.SetCookie(w, &cookie)

		db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("CookieBucket"))
			err = b.Put([]byte(cookie.Value), []byte(email))
			return err
		})
		return true
	}
	return false
}

func addUser(email string, password string) bool {
	check := []byte("")
	db, err := bolt.Open("goblog.db", 0600, nil)
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Println(err)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("UsersBucket"))
		check = b.Get([]byte(email))
		return nil
	})
	if check == nil {
		db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("UsersBucket"))
			err := b.Put([]byte(email), []byte(hashedPass))
			return err
		})
		return true
	} else {
		return false
	}
}

// http://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

func RandomString() string {
	b := make([]byte, 20)
	for i, cache, remain := 20-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func getUser(w http.ResponseWriter, r *http.Request) string {
	cookie, _ := r.Cookie("goblog")
	if cookie != nil {
		return getUserFromCookie(cookie.Value)
	}
	return ""
}

func getUserFromCookie(value string) string {
	servervalue := []byte("")
	db, err := bolt.Open("goblog.db", 0600, nil)
	if err != nil {
		fmt.Println(err)
	}
	defer db.Close()
	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("CookieBucket"))
		servervalue = b.Get([]byte(value))
		return nil
	})
	if servervalue != nil {
		return string(servervalue)
	}
	return ""
}

//func checkUrl(s string) (string, error) {
//	u, err := url.Parse(s)

//	if err != nil || u.Host == "" {
//		u, err = url.Parse("http://" + s)
//	}

//	return u.Host, err
//}

func main() {
	fmt.Println("Started server on port 1337")
	router := httprouter.New()
	router.GET("/", MainPage)
	router.POST("/login/", LoginHandler)
	router.GET("/login/", LoginPage)
	router.GET("/signup/", SignupPage)
	router.POST("/signup/", SignupHandler)
	router.GET("/admin/", AdminPage)
	router.POST("/admin/", BlogCreationHandler)
	router.GET("/logout/", LogoutHandler)
	router.GET("/error/:errorcode/", ErrorPage)
	router.ServeFiles("/css/*filepath", http.Dir(STATIC_FILES_DIR+"/css/"))
	router.ServeFiles("/js/*filepath", http.Dir(STATIC_FILES_DIR+"/js/"))
	router.ServeFiles("/img/*filepath", http.Dir(STATIC_FILES_DIR+"/img/"))
	router.ServeFiles("/fonts/*filepath", http.Dir(STATIC_FILES_DIR+"/fonts/"))
	log.Fatal(http.ListenAndServe(":1337", router))
}
