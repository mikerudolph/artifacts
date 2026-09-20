# Artifacts verification c70442ead37d

Classification: `none`

## Steps

- [x] doctor (4 ms) http://127.0.0.1:65326
- [x] REST create repository (15 ms) repository=agent-c70442ead37d/core-c70442ead37d
- [x] REST commit (190 ms) sha=ab6760f17bf05a2d546f5980ca544dc09841f7bf sequence=1
- [x] REST read (13 ms) path=README.md
- [x] create Git credential (2 ms) scope=write
- [x] Git clone (155 ms) cloned repository
- [x] Git verify REST commit (10 ms) head=ab6760f17bf05a2d546f5980ca544dc09841f7bf
- [x] Git commit and push (292 ms) sha=570f4b5797a3f554ff05cb2fa00a7e8c3089671f
- [x] REST readback (15 ms) path=result.txt
- [x] refs and WAL visibility (6 ms) refs=2 wal=2
- [x] delete repository (11 ms) final server state checked

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

- `git.log` `7e44e6bcfd231c2065bb89f8b5144a5bdf755136fbea8704e8abd9007411fb77` (435 bytes)
