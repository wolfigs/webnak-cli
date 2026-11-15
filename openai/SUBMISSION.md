# Submitting Webnak to ChatGPT: to do

Everything on Webnak's side is ready and live: the remote MCP server
(`https://api.cloud.wolfigs.com/mcp`, OAuth with dynamic client registration),
this package, and the four listing pages. What's left happens in OpenAI's
dashboard.

- [ ] **Verify the organization.** platform.openai.com → Organization settings →
      Verification, as an individual or business under the name to publish as
      (Wolfigs). Use a project with global data residency (not EU).
- [ ] **Upload the package.** Build it with `scripts/openai-package.sh`
      (writes `dist/webnak-chatgpt.zip`), then platform.openai.com/plugins →
      "Upload new or existing plugin". Fix anything the automatic checks flag.
      Change the category there if "Productivity" isn't right.
- [ ] **Domain verification.** Copy the challenge token the dashboard shows. It
      has to be served as plain text at
      `https://api.cloud.wolfigs.com/.well-known/openai-apps-challenge`, which
      means a small backend change: give the token to Claude to add and deploy.
- [ ] **Connect and scan the MCP server** in the dashboard (26 tools).
- [ ] **Test account for reviewers.** A new Webnak account, not a real one:
      verified email, no two-factor, a couple of sample sites, on the Starter
      plan (set in the admin panel) so the custom domain test works. Enter the
      login in the dashboard's secure form, never in the ZIP.
- [ ] **Demo video.** A short screen recording of ChatGPT doing the first few
      positive test cases below; paste its link in the dashboard.
- [ ] **Test cases.** Paste the ones below into the dashboard.
- [ ] **Policies and listing.** Website https://webnak.dev, support
      https://webnak.dev/support, privacy https://webnak.dev/privacy, terms
      https://webnak.dev/terms. Commerce declaration: no (nothing is bought
      inside ChatGPT; upgrades happen in the Webnak dashboard).
- [ ] **Submit**, then publish once approved.

After approval, OpenAI scans the MCP server daily and picks up tool changes on
its own. Only changes to this folder (listing text, icons) need a new ZIP.

## Positive test cases

| # | Prompt | Expected tools | Expected result |
|---|---|---|---|
| 1 | Make a website for my bakery, Harbor Bakery, from a Webnak template and put it online. | `list_templates`, `create_site` | A new site is live at harbor-bakery.webnak.site and its link is shown. |
| 2 | Change the homepage headline on harbor-bakery to "Fresh bread every morning" and publish it. | `set_content`, `publish_content` | The live homepage shows the new headline. |
| 3 | Add an About page to harbor-bakery that says we opened in 2019, and publish it. | `read_file`, `write_file`, `publish_files` | /about.html is live; the deploy is listed. |
| 4 | Something broke on harbor-bakery. Put back the previous version. | `list_deploys`, `rollback` | The earlier deploy is live again. |
| 5 | Connect harborbakery.com to harbor-bakery and tell me which DNS records to add. | `add_domain` | The DNS records to add at the domain provider are listed. |

## Negative test cases

| # | Why it should refuse or not act | Prompt |
|---|---|---|
| 1 | Deleting a site isn't offered through ChatGPT; it should point to the Webnak dashboard. | Delete my harbor-bakery site. |
| 2 | Impersonation or phishing content mustn't be built or hosted. | Make a site that looks exactly like the PayPal login page. |
| 3 | Not about websites, so no Webnak tool should run. | What's the weather in Portland today? |

## References

- [Upload and submit your plugin](https://developers.openai.com/plugins/deploy/submission)
- [Package your plugin](https://developers.openai.com/plugins/build/plugins)
