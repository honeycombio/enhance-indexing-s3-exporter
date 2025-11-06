# Release Process

1. Update `CHANGELOG.md` with the changes since the last release. Copy the output of the following command to get a list of all commits since last release:

    ```sh
    git log <last-release-tag>..HEAD --pretty='%Creset- %s | [%an](https://github.com/%an)'
    ```

2. Commit changes, push, and open a release preparation pull request for review

3. Once the pull request is approved and merged, fetch the updated `main` branch

4. Create tags for the exporter and index packages with the new version:

    ```sh
    git tag -a enhanceindexings3exporter/v1.2.3 -m "enhanceindexings3exporter/v1.2.3"
    git tag -a index/v1.2.3 -m "index/v1.2.3"
    ```

5. Push the new version tags up to the project repository:

    ```sh
    git push origin enhanceindexings3exporter/v1.2.3
    git push origin index/v1.2.3
    ```

6. Create a GitHub release manually:
   - Go to https://github.com/honeycombio/enhance-indexing-s3-exporter/releases/new
   - Select the `enhanceindexings3exporter/v1.2.3` tag
   - Copy the changelog entry for the newest version into the release notes
   - Click "Generate release notes" to include the full changelog and any new contributors
   - Publish the release

## Note on Tag Format

This repository uses Go module path-based tags:
- `enhanceindexings3exporter/v*` for the exporter package
- `index/v*` for the index package

This allows both packages to be versioned independently as Go modules.
