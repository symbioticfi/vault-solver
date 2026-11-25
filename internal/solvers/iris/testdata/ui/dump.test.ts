// Compiles the curator UI's Iris permissions and recipes for the fork-test account and writes
// them to IRIS_FIXTURE_OUT. generate.py runs it inside a curator-frontend checkout.
import { writeFileSync } from "node:fs";

import { expect, it } from "vitest";

import { newDraft, requestOf } from "@/pages/vault/components/views/allocations/components/adapter-detail/merkle/automation/builder/lib";
import { compilePermissions } from "@/shared/lib/merkle/operations/authoring/catalog/compile";
import { deploymentFor } from "@/shared/lib/merkle/operations/authoring/catalog/deployments";
import { recipeTemplate, recipeTemplatesDto } from "@/shared/lib/merkle/operations/authoring/recipes/definitions";
import { IRIS_DEBT_TOKENS } from "@/shared/lib/merkle/operations/authoring/recipes/iris-markets";
import { previewRecipe } from "@/shared/lib/merkle/operations/authoring/recipes/registry";
import { suggestRecipes } from "@/shared/lib/merkle/operations/authoring/recipes/suggest";
import { compileIrisBondClaim } from "@/shared/lib/merkle/operations/authoring/recipes/templates/iris-bond-claim";
import { verifyPermissionSet } from "@/shared/lib/merkle/verify";

const account = "0x0000000000000000000000000000000000acc001" as const;
// Aave Core and its USDC aToken and variable debt token; the fork test checks them on-chain.
const pool = "0x87870bca3f3fd6335c3f4ce8392d69350b4fa4e2" as const;
const aToken = "0x98c23e9d8f34fefb1b7bd6a91b7ff122f4e16f5c";
const variableDebt = "0x72e95b8931767c79ba4eee721354d6e99a61d004";
const zero = "0x0000000000000000000000000000000000000000";

async function strategy() {
    const scope = { account, adapter: account, chainId: 1 };
    const compiled = await compilePermissions(
        [
            { type: "iris_auction_offers", params: {} },
            { type: "iris_solver", params: { debtToken: IRIS_DEBT_TOKENS.USDC } },
            { type: "aave_supply", params: { pool, asset: IRIS_DEBT_TOKENS.USDC } },
        ],
        {
            ...scope,
            deployments: deploymentFor(1),
            vaultAsset: IRIS_DEBT_TOKENS.USDC,
            facts: {
                code: async () => "0x01",
                read: async (_target: string, signature: string) => {
                    if (signature.startsWith("getReserveData(")) {
                        return [[0n], 0n, 0n, 0n, 0n, 0n, 0, 0, aToken, zero, variableDebt, zero, 0n, 0n, 0n];
                    }
                    throw new Error(`unexpected read ${signature}`);
                },
                token: async () => ({ symbol: "USDC", decimals: 6 }),
            },
        },
    );
    expect(compiled.issues ?? []).toEqual([]);
    const set = { ...compiled, ...scope, root: compiled.root! };
    const verified = verifyPermissionSet(set, scope);
    if (!verified.ok) {
        throw new Error(verified.reason);
    }
    return { set, index: verified.index };
}

const json = (v: unknown) => JSON.stringify(v, (_k, x) => (typeof x === "bigint" ? x.toString() : x), 2);

it("dumps the iris fixture", async () => {
    const s = await strategy();
    const deployments = deploymentFor(1);
    const template = recipeTemplatesDto().find(({ id }) => id === "iris-offers")!;
    const suggestions = suggestRecipes(s, deployments).filter(({ template }) => template === "iris-offers");
    // Each market is its own executor, created the way the UI does: suggestion, edited limits, preview.
    const offer = (market: string, limits: Record<string, string>) => {
        const suggestion = suggestions.find(({ params }) => (params as { markets: string[] }).markets[0] === market)!;
        const draft = newDraft(s.set.root, template, suggestion);
        Object.assign(draft.params, limits);
        const request = requestOf(draft, template, s.set.root);
        expect(request.errors).toEqual({});
        const { compiled, issues } = previewRecipe(s, deployments, { template: recipeTemplate(template.id)!, params: request.body!.params });
        if (!compiled) {
            throw new Error(json(issues));
        }
        return { market, limits, recipe: compiled.recipe, recipeHash: compiled.recipeHash, calls: compiled.calls };
    };
    const withdrawBond = s.set.calls.find(({ fn }) => fn.startsWith("withdrawBond("))!;
    const claim = compileIrisBondClaim(account, deployments.iris!, withdrawBond, { account, chainId: 1 });
    writeFileSync(
        process.env.IRIS_FIXTURE_OUT!,
        json({
            uiRevision: process.env.IRIS_UI_REVISION,
            account,
            root: s.set.root,
            calls: s.set.calls.map(({ fn, target, data, dataMask, hash }) => ({ fn, target, data, dataMask, hash })),
            offers: [
                offer("wstETH-USDC-0", { maxPrincipal: "20000", maxTenorDays: "30", minAprPercent: "4" }),
                offer("wstETH-USDC-1", { maxPrincipal: "5000", maxTenorDays: "7", minAprPercent: "6.5" }),
            ],
            bondClaim: { recipe: claim.recipe, recipeHash: claim.recipeHash, calls: claim.calls },
        }) + "\n",
    );
});
