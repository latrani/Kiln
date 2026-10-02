# Releasing

> **AI-generated.** Claude wrote this page from Kiln's code and earlier README. If something here is wrong, please [open an issue](https://github.com/latrani/Kiln/issues).

Releases are built by GitHub Actions ([`.github/workflows/release.yml`](../.github/workflows/release.yml)) with [GoReleaser](https://goreleaser.com) ([`.goreleaser.yaml`](../.goreleaser.yaml)).

**From GitHub (no terminal needed):**

1. Open the repository's **Actions** tab and pick **release** in the list on the left.
2. Press **Run workflow**, leave the branch as `main`, type the version (like `v0.1.0`), and press the green **Run workflow** button.
3. Wait a few minutes. The workflow runs the tests, tags `main` with that version, builds every binary, and creates a **draft** release.
4. Open the **Releases** page, check the notes and files, and press **Publish release**. Until then, nothing is public.

If the run fails before tagging (tests failed, or the version is malformed or already used), nothing changes: fix it and run again. If it fails after tagging, delete the tag from the repository's **Tags** page (or `git push --delete origin v0.1.0`) before retrying.

**From a terminal:** pushing a version tag starts the same workflow.

```sh
git checkout main && git pull
git tag v0.1.0
git push origin v0.1.0
```

To try the build locally without publishing anything, run `goreleaser release --snapshot --clean` (the output goes to `dist/`).
