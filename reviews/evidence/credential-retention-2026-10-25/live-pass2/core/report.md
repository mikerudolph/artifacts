# Artifacts verification ed2fd19f9360

Classification: `none`

## Steps

- [x] doctor (1 ms) http://127.0.0.1:49145
- [x] REST create repository (2 ms) repository=agent-ed2fd19f9360/core-ed2fd19f9360
- [x] REST commit (17 ms) sha=f5a4297fcd4d6cd4d34b84207cb70cbdaf9a4b39 sequence=1
- [x] REST read (1 ms) path=README.md
- [x] create Git credential (0 ms) scope=write
- [x] Git clone (25 ms) cloned repository
- [x] Git verify REST commit (1 ms) head=f5a4297fcd4d6cd4d34b84207cb70cbdaf9a4b39
- [x] Git commit and push (35 ms) sha=310beda1c6982b3f9303f5c290469c3d3d2a6f27
- [x] REST readback (1 ms) path=result.txt
- [x] refs and WAL visibility (0 ms) refs=2 wal=2
- [x] delete repository (2 ms) final server state checked

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

- `git.log` `0313be8a3d1bd36547fb7e923429d953b704e78c600f4eccbaaf72abba46f72a` (391 bytes)
