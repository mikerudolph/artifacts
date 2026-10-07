# Artifacts verification ec5908433b43

Classification: `none`

## Steps

- [x] doctor (4 ms) http://127.0.0.1:58140
- [x] REST create repository (14 ms) repository=agent-ec5908433b43/core-ec5908433b43
- [x] REST commit (33 ms) sha=11739cd074d4148c15310e5f2338a55893f0a771 sequence=1
- [x] REST read (2 ms) path=README.md
- [x] create Git credential (0 ms) scope=write
- [x] Git clone (80 ms) cloned repository
- [x] Git verify REST commit (9 ms) head=11739cd074d4148c15310e5f2338a55893f0a771
- [x] Git commit and push (89 ms) sha=81df9a4068414479f090ab6f0bfd49a1085cefec
- [x] REST readback (3 ms) path=result.txt
- [x] refs and WAL visibility (1 ms) refs=2 wal=2
- [x] delete repository (3 ms) final server state checked

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

- `git.log` `bc76095cbafafb98f735b723a6a3cb8876f7659ffb2e01e343d9001bef710550` (435 bytes)
