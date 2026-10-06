# Artifacts verification bd77db1354c4

Classification: `none`

## Steps

- [x] doctor (5 ms) http://127.0.0.1:58612
- [x] REST create repository (11 ms) repository=agent-bd77db1354c4/core-bd77db1354c4
- [x] REST commit (54 ms) sha=ab61757f6373a6573dea325f9bfe403f058e11a0 sequence=1
- [x] REST read (7 ms) path=README.md
- [x] create Git credential (1 ms) scope=write
- [x] Git clone (169 ms) cloned repository
- [x] Git verify REST commit (30 ms) head=ab61757f6373a6573dea325f9bfe403f058e11a0
- [x] Git commit and push (347 ms) sha=e16614e48bca76123ea24c322032bb5eaebe0829
- [x] REST readback (7 ms) path=result.txt
- [x] refs and WAL visibility (3 ms) refs=2 wal=2
- [x] delete repository (5 ms) final server state checked

## Assertions

- [x] repository has Git remote remote present
- [x] REST content matches README.md content matched
- [x] Git sees REST content README matched
- [x] Git sees REST SHA SHA matched
- [x] REST content matches result.txt content matched
- [x] main ref exposes Git push refs/heads/main matched
- [x] WAL exposes REST and Git publications at least two publications
- [x] repository absent after cleanup GET returned not found

## Artifacts

- `git.log` `4e56840b48113aa670ba545050d8a0cd057aff34c94d9edaaef9035d4e49d148` (435 bytes)
