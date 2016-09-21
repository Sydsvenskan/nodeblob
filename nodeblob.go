package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

// Package supports barebones package info
type Package struct {
	Name            string
	DevDependencies map[string]string `json:"devDependencies"`
	Dependencies    map[string]string `json:"dependencies"`
}

// Hash calculates a has off the dependencies
func (p *Package) Hash() string {
	var deps []string

	for module, semver := range p.Dependencies {
		deps = append(deps, fmt.Sprintf("%s: %s", module, semver))
	}
	for module, semver := range p.DevDependencies {
		deps = append(deps, fmt.Sprintf("%s: %s", module, semver))
	}

	sort.Strings(deps)
	hash := sha256.New()
	for _, dep := range deps {
		_, _ = hash.Write([]byte(dep))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

type options struct {
	Directory string
	Bucket    string
	Prefix    string
}

func main() {
	bucket := flag.String("bucket", "", "the bucket to store cached modules in")
	prefix := flag.String("path", "node_modules", "the S3 object path prefix")
	flag.Parse()

	if *bucket == "" {
		fmt.Fprintln(os.Stderr, "missing required bucket flag")
		os.Exit(1)
	}

	directory, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get current working directory")
		os.Exit(1)
	}
	args := flag.Args()
	if len(args) > 0 {
		directory = args[0]
	}

	opt := options{
		Directory: directory,
		Bucket:    *bucket,
		Prefix:    *prefix,
	}

	if err := process(opt); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func process(opt options) error {
	packagePath := path.Join(opt.Directory, "package.json")
	packageFile, err := os.Open(packagePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open package file %q", packagePath)
	}
	defer packageFile.Close()

	var pkg Package
	decoder := json.NewDecoder(packageFile)
	if err := decoder.Decode(&pkg); err != nil {
		return errors.Wrap(err, "failed to decode package file")
	}

	archiveName := fmt.Sprintf(
		"%s-%s-%s-%s.tar.gz",
		pkg.Name, pkg.Hash(),
		runtime.GOOS, runtime.GOARCH,
	)
	svc := s3.New(session.New())

	err = getCachedModules(svc, archiveName, opt)
	if err == nil {
		return nil
	}
	fmt.Fprintln(os.Stderr, "Could not get cached modules", err.Error())

	install := exec.Command("npm", "install", "-q")
	install.Stderr = os.Stderr
	install.Stdout = os.Stdout
	install.Dir = opt.Directory

	if err := install.Run(); err != nil {
		return errors.Wrap(err, "failed to install npm modules")
	}

	localPath := path.Join(os.TempDir(), archiveName)
	archive := exec.Command("tar", "-czf",
		localPath, "node_modules")
	archive.Stderr = os.Stderr
	archive.Stdout = os.Stdout
	archive.Dir = opt.Directory

	if err := archive.Run(); err != nil {
		return errors.Wrap(err, "failed to create module archive")
	}

	archiveFile, err := os.Open(localPath)
	if err != nil {
		return errors.Wrap(err, "failed to open module archive for upload")
	}

	uploadParams := s3.PutObjectInput{
		Body:   archiveFile,
		Key:    aws.String(path.Join(opt.Prefix, archiveName)),
		Bucket: aws.String("concourse-state"),
	}
	if _, err := svc.PutObject(&uploadParams); err != nil {
		return errors.Wrap(err, "failed to upload module archive")
	}

	return nil
}

func getCachedModules(svc *s3.S3, archiveName string, opt options) error {
	key := path.Join(opt.Prefix, archiveName)
	params := &s3.GetObjectInput{
		Bucket: aws.String(opt.Bucket),
		Key:    aws.String(key),
	}
	cached, err := svc.GetObject(params)
	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		return errors.Wrapf(err,
			"failed to get cached module archive %q", key)
	}
	defer cached.Body.Close()

	localPath := path.Join(os.TempDir(), archiveName)
	localFile, err := os.Create(localPath)
	if err != nil {
		return errors.Wrap(err, "failed to create temporary cache file")
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, cached.Body); err != nil {
		return errors.Wrap(err, "failed to write temporary cache file")
	}

	cmd := exec.Command("tar", "-C", opt.Directory, "-xzf", localPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to extract node_modules from cache file")
	}

	return nil
}
