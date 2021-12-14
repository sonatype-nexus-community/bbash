module github.com/sonatype-nexus-community/bbash

go 1.16

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/golang-migrate/migrate/v4 v4.14.1
	github.com/joho/godotenv v1.3.0
	github.com/labstack/echo/v4 v4.2.2
	github.com/labstack/gommon v0.3.0
	github.com/stretchr/testify v1.7.0
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
