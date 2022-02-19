module github.com/sonatype-nexus-community/bbash

go 1.17

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/golang-migrate/migrate/v4 v4.14.1
	github.com/joho/godotenv v1.3.0
	github.com/labstack/echo/v4 v4.2.2
	github.com/labstack/gommon v0.3.0
	github.com/stretchr/testify v1.7.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.0 // indirect
	github.com/lib/pq v1.8.0 // indirect
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.1 // indirect
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777 // indirect
	golang.org/x/sys v0.0.0-20210124154548-22da62e12c0c // indirect
	golang.org/x/text v0.3.5 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

// fix: CVE-2021-3121 in pkg:golang/github.com/gogo/protobuf@1.3.1
replace github.com/dhui/dktest => github.com/dhui/dktest v0.3.4

// fix: CVE-2021-20329 in pkg:golang/go.mongodb.org/mongo-driver@v1.1.0
replace go.mongodb.org/mongo-driver => go.mongodb.org/mongo-driver v1.5.1

// fix: CVE-2021-21334 in pkg:golang/github.com/containerd/containerd@v1.4.3
// fix: CVE-2021-32760 in github.com/containerd/containerd v1.4.4
// fix: CVE-2021-41103 in github.com/containerd/containerd v1.4.8
// fix: CVE-2021-41190 in github.com/containerd/containerd v1.4.11
replace github.com/containerd/containerd => github.com/containerd/containerd v1.4.12

// fix: SONATYPE-2019-0702 in github.com/gobuffalo/packr/v2 v2.2.0
replace github.com/gobuffalo/packr/v2 => github.com/gobuffalo/packr/v2 v2.3.2

// fix: CVE-2020-15114 in etcd v3.3.10
replace github.com/coreos/etcd => github.com/coreos/etcd v3.3.24+incompatible

// fix: sonatype-2021-0853 in github.com/jackc/pgproto3 v1.1.0
replace github.com/jackc/pgproto3 => github.com/jackc/pgproto3/v2 v2.1.1

// fix vulnerability: CVE-2021-38561 in golang.org/x/text v0.3.5
replace golang.org/x/text => golang.org/x/text v0.3.7
