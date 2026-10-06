# Artifacts verification ab2654456af1

Classification: `none`

## Steps

- [x] doctor (2 ms) http://127.0.0.1:58612
- [x] REST create repository (4 ms) repository=agent-ab2654456af1/core-ab2654456af1
- [x] REST commit (30 ms) sha=1e10935894129470397be8e428aa7f288e4499a0 sequence=1
- [x] REST read (5 ms) path=README.md
- [x] create Git credential (1 ms) scope=write
- [x] Git clone (135 ms) cloned repository
- [x] Git verify REST commit (44 ms) head=1e10935894129470397be8e428aa7f288e4499a0
- [x] Git commit and push (288 ms) sha=5df05ae58da76093b2170e777834012dab80f3c3
- [x] REST readback (10 ms) path=result.txt
- [x] refs and WAL visibility (3 ms) refs=2 wal=2
- [x] generate binary fixture (105 ms) bytes=104857553 sha256=9cc20c310bf609e8e1233b1b9b2d17101ca7b2ff172b278fcdb4b1bc0b480653
- [x] multipart commit (8233 ms) status=201 sha=c44ad2d1eafe31da6e7ef5fc6e02b0664e2e93a0 bytes=104857600 sequence=3
- [x] binary REST readback (4340 ms) sha=c44ad2d1eafe31da6e7ef5fc6e02b0664e2e93a0 bytes=104857553
- [x] Git verifies binary and mode (4971 ms) binary.sh executable
- [x] Git changes binary (14239 ms) sha=fd5ce8e439d03d6207d122cc80c0e50021b84833
- [x] REST reads Git binary (2248 ms) sha=fd5ce8e439d03d6207d122cc80c0e50021b84833 bytes=104857554
- [x] multipart retry after Git advancement (3144 ms) publications=4
- [x] delete repository (14 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [x] multipart commit accepted HTTP 201; expected 201
- [x] REST binary checksum matches c44ad2d1eafe31da6e7ef5fc6e02b0664e2e93a0 byte count and SHA-256 matched
- [x] Git binary checksum matches REST SHA-256 matched
- [x] Git sees executable mode 100755
- [x] REST binary checksum matches fd5ce8e439d03d6207d122cc80c0e50021b84833 byte count and SHA-256 matched
- [x] binary retry returns original publication original SHA and sequence
- [x] changed keyed binary request conflicts HTTP 409
- [x] stale binary expected head conflicts HTTP 409
- [x] binary retries publish no additional commits exactly four REST/Git publications
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `5fa95db914e63afce160e6ddeec7437ab0966e61fe4131d85e2dd510b7db4455` (843 bytes)
