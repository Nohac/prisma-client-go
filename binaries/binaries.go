package binaries

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/prisma/prisma-client-go/binaries/platform"
	"github.com/prisma/prisma-client-go/logger"
)

// PrismaVersion is a hardcoded version of the Prisma CLI.
const PrismaVersion = "2.0.0-beta.2"

// EngineVersion is a hardcoded version of the Prisma Engine.
// The versions can be found under https://github.com/prisma/prisma-engine/commits/master.
const EngineVersion = "76857c35ba1e1764dd5473656ecbbb2f739e1822"

// PrismaURL points to an S3 bucket URL where the CLI binaries are stored.
var PrismaURL = "https://prisma-photongo.s3-eu-west-1.amazonaws.com/%s-%s-%s.gz"

// EngineURL points to an S3 bucket URL where the Prisma engines are stored.
var EngineURL = "https://binaries.prisma.sh/master/%s/%s/%s.gz"

// init overrides URLs if env variables are specific for debugging purposes and to
// be able to provide a fallback if the links above should go down
func init() {
	if prismaURL, ok := os.LookupEnv("PRISMA_CLI_URL"); ok {
		PrismaURL = prismaURL
	}
	if engineURL, ok := os.LookupEnv("PRISMA_ENGINE_URL"); ok {
		EngineURL = engineURL
	}
}

// PrismaCLIName returns the local file path of where the CLI lives
func PrismaCLIName() string {
	variation := platform.Name()
	return fmt.Sprintf("prisma-cli-%s", variation)
}

var baseDirName = path.Join("prisma", "binaries")

// GlobalTempDir returns the path of where the engines live
// internally, this is the global temp dir
func GlobalTempDir() string {
	temp := os.TempDir()
	logger.Debug.Printf("temp dir: %s", temp)

	return path.Join(temp, baseDirName, "engines", EngineVersion)
}

// GlobalCacheDir returns the path of where the CLI lives
// internally, this is the global temp dir
func GlobalCacheDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		panic(fmt.Errorf("could not read user cache dir: %w", err))
	}

	logger.Debug.Printf("global cache dir: %s", cache)

	return path.Join(cache, baseDirName, "cli", PrismaVersion)
}

func FetchEngine(toDir string, engineName string, binaryPlatformName string) error {
	logger.Debug.Printf("checking %s...", engineName)

	to := path.Join(toDir, fmt.Sprintf("prisma-%s-%s", engineName, binaryPlatformName))

	url := fmt.Sprintf(EngineURL, EngineVersion, binaryPlatformName, engineName)

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		logger.Debug.Printf("%s is cached", to)
		return nil
	}

	logger.Debug.Printf("%s is missing, downloading...", engineName)

	if err := download(url, to); err != nil {
		return fmt.Errorf("could not download %s to %s: %w", url, to, err)
	}

	logger.Debug.Printf("%s done", engineName)

	return nil
}

// FetchNative fetches the Prisma binaries needed for the generator to a given directory
func FetchNative(toDir string) error {
	if toDir == "" {
		return fmt.Errorf("toDir must be provided")
	}

	if !strings.HasPrefix(toDir, "/") {
		return fmt.Errorf("toDir must be absolute")
	}

	if err := DownloadCLI(toDir); err != nil {
		return fmt.Errorf("could not download engines: %w", err)
	}

	engines := []string{
		"query-engine",
		"migration-engine",
		"introspection-engine",
	}

	for _, e := range engines {
		if _, err := DownloadEngine(e, toDir); err != nil {
			return fmt.Errorf("could not download engines: %w", err)
		}
	}

	return nil
}

func DownloadCLI(toDir string) error {
	cli := PrismaCLIName()
	to := path.Join(toDir, cli)
	url := fmt.Sprintf(PrismaURL, "prisma-cli", PrismaVersion, platform.Name())

	if _, err := os.Stat(to); os.IsNotExist(err) {
		logger.Debug.Printf("prisma cli doesn't exist, fetching...")

		if err := download(url, to); err != nil {
			return fmt.Errorf("could not download %s to %s: %w", url, to, err)
		}

		logger.Debug.Printf("prisma cli fetched successfully.")
	} else {
		logger.Debug.Printf("prisma cli is cached")
	}

	return nil
}

func DownloadEngine(name string, toDir string) (file string, err error) {
	binaryName := platform.BinaryPlatformName()

	logger.Debug.Printf("checking %s...", name)

	to := path.Join(toDir, fmt.Sprintf("prisma-%s-%s", name, binaryName))

	url := fmt.Sprintf(EngineURL, EngineVersion, binaryName, name)

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		logger.Debug.Printf("%s is cached", to)
		return to, nil
	}

	logger.Debug.Printf("%s is missing, downloading...", name)

	startDownload := time.Now()
	if err := download(url, to); err != nil {
		return "", fmt.Errorf("could not download %s to %s: %w", url, to, err)
	}

	logger.Debug.Printf("download() took %s", time.Since(startDownload))

	logger.Debug.Printf("%s done", name)

	return to, nil
}

func download(url string, to string) error {
	if err := os.MkdirAll(path.Dir(to), os.ModePerm); err != nil {
		return fmt.Errorf("could not run MkdirAll on path %s: %w", to, err)
	}

	// copy to temp file first
	dest := to + ".tmp"

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("could not get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		out, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("received code %d from %s: %+v", resp.StatusCode, url, string(out))
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("could not create %s: %w", dest, err)
	}
	defer out.Close()

	if err := os.Chmod(dest, os.ModePerm); err != nil {
		return fmt.Errorf("could not chmod +x %s: %w", url, err)
	}

	g, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("could not create gzip reader: %w", err)
	}
	defer g.Close()

	if _, err := io.Copy(out, g); err != nil {
		return fmt.Errorf("could not copy %s: %w", url, err)
	}

	// temp file is ready, now copy to the original destination
	if err := copyFile(dest, to); err != nil {
		return fmt.Errorf("copy temp file: %w", err)
	}

	return nil
}

func copyFile(from string, to string) error {
	input, err := ioutil.ReadFile(from)
	if err != nil {
		return fmt.Errorf("readfile: %w", err)
	}

	err = ioutil.WriteFile(to, input, os.ModePerm)
	if err != nil {
		return fmt.Errorf("writefile: %w", err)
	}

	return nil
}
