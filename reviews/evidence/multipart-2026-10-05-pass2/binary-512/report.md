# Artifacts verification e3be4abb43fa

Classification: `none`

## Steps

- [x] doctor (2 ms) http://127.0.0.1:58612
- [x] REST create repository (8 ms) repository=agent-e3be4abb43fa/core-e3be4abb43fa
- [x] REST commit (97 ms) sha=c9fc402b6f9c35693f9487499fee280527008467 sequence=1
- [x] REST read (8 ms) path=README.md
- [x] create Git credential (3 ms) scope=write
- [x] Git clone (123 ms) cloned repository
- [x] Git verify REST commit (16 ms) head=c9fc402b6f9c35693f9487499fee280527008467
- [x] Git commit and push (247 ms) sha=161be46d12e740eb494462a547f89c8a8c212fbe
- [x] REST readback (8 ms) path=result.txt
- [x] refs and WAL visibility (3 ms) refs=2 wal=2
- [x] generate binary fixture (1108 ms) bytes=536870865 sha256=2f2440e0986260c7abb18789917c4b628ac75bff9465170dbed946400defff15
- [x] multipart commit (40062 ms) status=201 sha=f7cbc025b25c66b1af9a5732eb509002a5215808 bytes=536870912 sequence=3
- [x] binary REST readback (26277 ms) sha=f7cbc025b25c66b1af9a5732eb509002a5215808 bytes=536870865
- [x] Git verifies binary and mode (30311 ms) binary.sh executable
- [x] Git changes binary (45748 ms) sha=7de30a5717c0c3ccae953ddcd1e1c5a49f13002b
- [x] REST reads Git binary (9251 ms) sha=7de30a5717c0c3ccae953ddcd1e1c5a49f13002b bytes=536870866
- [x] multipart retry after Git advancement (7934 ms) publications=4
- [x] delete repository (51 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [x] multipart commit accepted HTTP 201; expected 201
- [x] REST binary checksum matches f7cbc025b25c66b1af9a5732eb509002a5215808 byte count and SHA-256 matched
- [x] Git binary checksum matches REST SHA-256 matched
- [x] Git sees executable mode 100755
- [x] REST binary checksum matches 7de30a5717c0c3ccae953ddcd1e1c5a49f13002b byte count and SHA-256 matched
- [x] binary retry returns original publication original SHA and sequence
- [x] changed keyed binary request conflicts HTTP 409
- [x] stale binary expected head conflicts HTTP 409
- [x] binary retries publish no additional commits exactly four REST/Git publications
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `8416d4d28b3fffc982845283f5fd290f380ec9771220348882cd132f6c598962` (958 bytes)
