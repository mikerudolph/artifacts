export interface DiagramStep {
  label: string;
  title: string;
  description: string;
}

export const stories = {
  publication: {
    number: '01',
    category: 'Publication',
    title: 'Durable bytes. Then visible refs.',
    summary: 'A publication stores its immutable pack and index before one Postgres transaction records the publication and updates refs. Readers cannot observe a ref pointing to an unpublished pack.',
    note: 'This is Artifacts’ pack publication log, not a visualization of Postgres’ internal WAL. SHAs are abbreviated; sequence 42 is illustrative.',
    steps: [
      { label: 'Prepare', title: 'Prepare the immutable objects', description: 'Git validates the received data in the local repository cache. Artifacts repacks the repository into a .pack file and its .idx index. Published main still points to a1b2c3.' },
      { label: 'Store', title: 'Make both files durable', description: 'The pack and index are uploaded to object storage first. Their presence alone does not publish a new version: readers still resolve the old ref.' },
      { label: 'Validate', title: 'Check the expected state', description: 'A Postgres transaction locks the repository and checks its status, expected WAL sequence, and every expected old ref. A conflict aborts publication; uploaded bytes remain unreachable.' },
      { label: 'Publish', title: 'Publish the log and refs atomically', description: 'The transaction inserts pack_wal and ref-update records, advances the sequence, and changes main to d4e5f6. These changes become visible together when the transaction commits.' },
      { label: 'Confirm', title: 'Only now acknowledge success', description: 'The service returns success after publication commits. REST reads and Git fetches can resolve the new version because its object bytes are already durable.' },
    ],
  },
  storage: {
    number: '02',
    category: 'Storage & recovery',
    title: 'Lose the cache. Keep the repository.',
    summary: 'Postgres and object storage are durable. The local bare Git repository is a disposable materialization. After losing it, Artifacts installs a checkpoint and subsequent packs, then restores published refs.',
    note: 'A repository without fork lineage is shown. Forked repositories load captured parent history first. Object storage can be a local filesystem or S3; the key layout is the same.',
    steps: [
      { label: 'Layers', title: 'Three layers, two sources of truth', description: 'Postgres owns refs and publication metadata. Object storage holds immutable pack/index pairs under {account}/{repo}/pack/. The local cache is a bare Git repository, not your only copy.' },
      { label: 'Evict', title: 'A missing cache is recoverable', description: 'If the cache is missing, stale, or corrupt, the server reconstructs it. Neither the durable packs nor the published refs disappear when the local cache is removed.' },
      { label: 'Base', title: 'Install the newest usable checkpoint', description: 'Metadata identifies a checkpoint at sequence 40. Its pack and index are downloaded into the cache’s objects/pack/ directory, with the pack checksum verified.' },
      { label: 'Replay', title: 'Install packs after the checkpoint', description: 'The server follows WAL entries 41 and 42 and installs their immutable pack/index pairs in sequence. These entries locate Git packs; they are not a log of individual file edits.' },
      { label: 'Refs', title: 'Restore refs, then serve the repository', description: 'Current refs come from Postgres, and symbolic HEAD is set to the configured default branch. The rebuilt cache represents sequence 42 and can serve ordinary file reads and Git operations.' },
    ],
  },
  objects: {
    number: '01',
    category: 'The repository model',
    title: 'A ref moves. A snapshot does not.',
    summary: 'A branch ref points to a commit. Each commit points to a root tree; tree entries name file blobs or other trees. A new version adds immutable objects, can reuse unchanged blobs, and moves a ref without changing the old snapshot.',
    note: 'A two-file repository is shown. Directories add more tree nodes. Edges point to referenced objects; B’s parent is A. A and B stand in for immutable commit SHAs.',
    steps: [
      { label: 'Ref', title: 'Start with a name, resolve a commit', description: 'main is a movable ref pointing to commit A. A commit is an immutable snapshot plus parent history, author, and message. Store its SHA when your application needs an exact version.' },
      { label: 'Tree', title: 'A tree gives bytes their paths', description: 'A’s root tree maps report.md and session.json to blobs. Blobs hold file bytes; the tree holds names and structure. Artifacts does not interpret your JSON or impose an application schema.' },
      { label: 'Edit', title: 'An edit creates a new snapshot', description: 'Updating report.md produces a new blob, a new root tree, and commit B with A as its parent. The old commit, tree, and file bytes stay unchanged. Object creation is shown separately from ref publication.' },
      { label: 'Reuse', title: 'Unchanged bytes can be shared', description: 'session.json did not change, so both trees reference the same blob. A snapshot is a complete view of your files, but unchanged objects do not need a new identity.' },
      { label: 'Advance', title: 'Publish by moving the ref', description: 'After publication, main points to B. Reading by main follows the new snapshot; reading by A’s SHA still gives the old one. This is why a branch name is not a stable version identifier.' },
    ],
  },
  forks: {
    number: '02',
    category: 'Independent histories',
    title: 'A shared past. Independent futures.',
    summary: 'A snapshot fork creates a new repository with its own refs and credential. Both repositories initially resolve the captured commit B and share its immutable history. Later commits in either repository do not move the other repository’s refs.',
    note: 'The fork captures the parent’s current published state, not an arbitrary historical SHA. Both repositories stay in the same account and namespace. Arrows follow refs or commit parents.',
    steps: [
      { label: 'Source', title: 'Begin with a published history', description: 'The source repository’s main points to B, whose parent is A. These commits and their file objects are immutable.' },
      { label: 'Capture', title: 'Capture a point in time', description: 'Forking records the source’s current publication sequence and copies selected refs in one metadata transaction. The child’s main also resolves B. Object bytes are shared, not copied; the child gets its own credential.' },
      { label: 'Parent', title: 'The parent moves on', description: 'The source publishes C and moves its main. The child still resolves B: a fork is a snapshot, not a subscription to future parent writes.' },
      { label: 'Child', title: 'The child writes its own next chapter', description: 'The child publishes X with B as its parent and advances its own main. The source stays at C. Bringing work back requires your application’s workflow or an explicitly coordinated Git merge.' },
    ],
  },
} satisfies Record<string, {
  number: string;
  category: string;
  title: string;
  summary: string;
  note: string;
  steps: DiagramStep[];
}>;

export type StoryName = keyof typeof stories;
