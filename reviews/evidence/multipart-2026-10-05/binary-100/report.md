# Artifacts verification ea8868ea2284

Classification: `none`

## Steps

- [x] doctor (1 ms) http://127.0.0.1:57858
- [x] REST create repository (3 ms) repository=agent-ea8868ea2284/core-ea8868ea2284
- [x] REST commit (19 ms) sha=e52d737a875da4140c05a9143c2f6895eda8d199 sequence=1
- [x] REST read (2 ms) path=README.md
- [x] create Git credential (0 ms) scope=write
- [x] Git clone (59 ms) cloned repository
- [x] Git verify REST commit (18 ms) head=e52d737a875da4140c05a9143c2f6895eda8d199
- [x] Git commit and push (160 ms) sha=89e4327261486ff1bab4d8eac55bd40634fb1910
- [x] REST readback (3 ms) path=result.txt
- [x] refs and WAL visibility (1 ms) refs=2 wal=2
- [x] generate binary fixture (104 ms) bytes=104857553 sha256=9c91a087d13a3d8ff390ee26ef8f06e16f30fdc2e6c329b0a6206299901cb84c
- [x] multipart commit (4913 ms) sha=93ca1b68e8b74f3ab163423ece0197777839c5d3 bytes=104857600 sequence=3
- [x] binary REST readback (2378 ms) sha=93ca1b68e8b74f3ab163423ece0197777839c5d3 bytes=104857553
- [x] Git verifies binary and mode (4223 ms) binary.sh executable
- [x] Git changes binary (18674 ms) sha=00cb2678329ff66f1f8916dedfe213491b97f7ec
- [x] REST reads Git binary (3145 ms) sha=00cb2678329ff66f1f8916dedfe213491b97f7ec bytes=104857554
- [x] multipart retry after Git advancement (3518 ms) publications=4
- [x] delete repository (30 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [x] multipart commit accepted HTTP 201
- [x] REST binary checksum matches 93ca1b68e8b74f3ab163423ece0197777839c5d3 byte count and SHA-256 matched
- [x] Git binary checksum matches REST SHA-256 matched
- [x] Git sees executable mode 100755
- [x] REST binary checksum matches 00cb2678329ff66f1f8916dedfe213491b97f7ec byte count and SHA-256 matched
- [x] binary retry returns original publication original SHA and sequence
- [x] changed keyed binary request conflicts HTTP 409
- [x] stale binary expected head conflicts HTTP 409
- [x] binary retries publish no additional commits exactly four REST/Git publications
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `08eac5771d12a1b8eff56fb413e031dde891bea69827e1d941c31a08aad68beb` (843 bytes)
