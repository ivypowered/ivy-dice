<?php
require_once __DIR__ . "/util.php";

$user = authenticate();
$max_bet =
    call_backend([
        "action" => "max_bet",
    ])["maxBetCents"] / 100.0;

// Handle bet submission
$bet_result = null;
$bet_error = null;
if (isset($_POST["client_seed"]) && $user["logged_in"]) {
    try {
        $bet_result = call_backend([
            "action" => "bet",
            "message" => $user["message"],
            "signature" => $user["signature"],
            "wagerCents" => (int) (floatval($_POST["bet_amount"]) * 100),
            "rollUnder" => $_POST["is_under"] === "true" ? true : false,
            "threshold" => (int) (floatval($_POST["threshold"]) * 100),
            "clientSeed" => $_POST["client_seed"],
        ]);

        // Update user balance after bet
        if (isset($bet_result["deltaCents"])) {
            $user["balance"] += $bet_result["deltaCents"] / 100.0;
        }
    } catch (Exception $e) {
        $bet_error = $e->getMessage();
    }
}

$recent_bets = [];
if ($user["logged_in"]) {
    try {
        $recent_bets = call_backend([
            "action" => "bet_list",
            "message" => $user["message"],
            "signature" => $user["signature"],
            "count" => 10,
            "skip" => 0,
        ]);
    } catch (Exception $e) {
        $recent_bets = [];
    }
}
?>

<!doctype html>
<html lang="en">
    <head>
        <meta charset="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>Ivy Dice</title>
        <link rel="stylesheet" type="text/css" href="styles.css">
        <style>
            /* ... (styles unchanged) ... */
        </style>
    </head>
    <body class="bg-gray-900 text-white min-h-screen">
        <!-- Main Content -->
        <main class="container mx-auto p-4">
            <div class="max-w-4xl mx-auto">
                <div class="flex justify-between items-center text-sm text-gray-400 mb-4">
                    <div>
                        Balance: <?= icon(
                            "dice-6",
                            "h-4 w-4 inline
                        align-middle text-gray-400"
                        ) ?>
                        <span id="user-balance"><?= number_format(
                            $user["balance"],
                            2
                        ) ?></span>
                    </div>

                    <?php if ($user["logged_in"]): ?>
                        <details id="account-dropdown" class="flex relative">
                            <summary
                                class="cursor-pointer font-bold hover:text-gray-200 list-none select-none mr-8"
                            >
                                <?= substr($user["id"], 0, 6) ?> ▼
                            </summary>
                            <div
                                class="absolute right-0 mt-6 w-48 bg-gray-900 rounded-md shadow-lg py-1 z-50 border border-gray-700"
                            >
                                <a
                                    href="/deposit.php"
                                    class="block px-4 py-2 hover:bg-gray-800"
                                    >Deposit</a
                                >
                                <a
                                    href="/withdraw.php"
                                    class="block px-4 py-2 hover:bg-gray-800"
                                    >Withdraw</a
                                >
                                <hr class="border-gray-700 my-1" />
                                <a
                                    href="#"
                                    id="logout-btn"
                                    class="block px-4 py-2 hover:bg-gray-800"
                                    >Logout</a
                                >
                            </div>
                        </details>
                    <?php else: ?>
                        <button
                            id="login-btn"
                            class="px-4 py-2 bg-gray-600 text-gray-300 rounded-md hover:bg-gray-500 transition-colors mr-8 disabled"
                        >Return to Ivy to log in</button>
                    <?php endif; ?>
                </div>

                <form method="post" class="select-none">
                    <div class="grid grid-cols-2 gap-4 mb-6">
                        <!-- Bet Amount -->
                        <div>
                            <div class="flex">
                                <div class="relative flex-1">
                                    <span
                                        class="absolute left-3 top-1/2 -translate-y-1/2"
                                    >
                                        <?= icon(
                                            "dice-6",
                                            "h-4 w-4
                                        text-gray-400"
                                        ) ?>
                                    </span>
                                    <input
                                        type="number"
                                        id="bet-amount"
                                        name="bet_amount"
                                        class="w-full pl-10 pr-3 py-2 bg-gray-800 border border-gray-700 rounded-l-md text-white placeholder-gray-500 focus:outline-none"
                                        placeholder="Bet"
                                        min="0"
                                        step="0.01"
                                        required
                                        value="<?= $_POST["bet_amount"] ??
                                            "" ?>"
                                    />
                                </div>
                                <button
                                    type="button"
                                    id="max-btn"
                                    class="px-4 py-2 bg-gray-700 border border-gray-600 text-gray-300 font-bold hover:bg-gray-600 transition-colors rounded-r-md"
                                >
                                    MAX
                                </button>
                            </div>
                        </div>

                        <!-- Profit Display -->
                        <div>
                            <div class="flex">
                                <div class="relative flex-1">
                                    <span
                                        class="absolute left-3 top-1/2 -translate-y-1/2"
                                    >
                                        <?= icon(
                                            "dice-6",
                                            "h-4 w-4
                                        text-gray-400"
                                        ) ?>
                                    </span>
                                    <input
                                        type="text"
                                        class="w-full pl-10 pr-3 py-2 bg-gray-800 border border-gray-700 rounded-l-md text-white placeholder-gray-500"
                                        placeholder="Profit"
                                        id="profit"
                                        readonly
                                    />
                                </div>
                                <button
                                    type="button"
                                    class="px-4 py-2 bg-gray-700 border border-gray-600 text-gray-300 font-bold rounded-r-md cursor-not-allowed opacity-60"
                                    disabled
                                >
                                    PROFIT
                                </button>
                            </div>
                        </div>
                    </div>

                    <!--- preserve parentOrigin for Ivy -->
                    <? if (isset($_GET["parentOrigin"])) { ?>
                        <input type="hidden" name="parentOrigin" value="<?= $_GET[
                            "parentOrigin"
                        ] ?? "" ?>" />
                    <? } ?>
                    <input type="hidden" id="client-seed" name="client_seed" value="" />
                    <input type="hidden" id="is-under" name="is_under" value="<?= isset(
                        $_POST["is_under"]
                    )
                        ? $_POST["is_under"]
                        : "true" ?>" />
                    <input type="hidden" id="threshold" name="threshold" value="<?= isset(
                        $_POST["threshold"]
                    )
                        ? $_POST["threshold"]
                        : 49.5 ?>" />

                    <!-- Win Chance & Payout -->
                    <div class="grid grid-cols-3 gap-4 mb-6">
                        <div class="bg-gray-800 rounded-md p-4 text-center">
                            <h6 class="text-xs text-gray-400 mb-2">
                                WIN CHANCE
                            </h6>
                            <div
                                class="flex flex-row justify-center text-2xl gap-2"
                            >
                                <input
                                    type="number"
                                    id="win-chance"
                                    class="w-16 flex bg-transparent text-white text-right focus:outline-none"
                                    min="1"
                                    max="98.02"
                                    step="0.01"
                                    value="49.5"
                                    required
                                />
                                <span class="text-white flex">%</span>
                            </div>
                        </div>

                        <button
                            type="button"
                            class="w-full h-full bg-gray-700 hover:bg-gray-600 rounded-md text-white py-4 text-center transition-colors"
                            id="roll-uo"
                        >
                            <div class="text-gray-400">
                                ROLL
                                <span id="roll-uo-text">UNDER</span>
                            </div>
                            <div class="text-2xl" id="roll-amount">49.50</div>
                            <div class="text-sm text-gray-400">TO WIN</div>
                        </button>

                        <div
                            class="bg-gray-800 rounded-md p-4 text-center relative"
                        >
                            <h6 class="text-xs text-gray-400 mb-2">PAYOUT</h6>
                            <div
                                class="flex flex-row justify-center text-2xl gap-2"
                            >
                                <input
                                    type="number"
                                    id="payout"
                                    class="w-16 flex bg-transparent text-white text-right focus:outline-none"
                                    min="1"
                                    max="99.0"
                                    step="0.01"
                                    value="2.00"
                                    required
                                />
                                <span class="text-white flex">×</span>
                            </div>
                        </div>
                    </div>

                    <!-- Slider -->
                    <div class="relative h-10">
                        <div
                            class="absolute w-full h-4 bg-gray-800 rounded-full overflow-hidden z-0"
                        >
                            <div
                                class="h-full bg-gray-600"
                                id="bet-slider-bg"
                                style="width: 49.5%"
                            ></div>
                        </div>
                        <input
                            type="range"
                            id="bet-slider"
                            min="1"
                            max="98.02"
                            step="0.01"
                            value="49.5"
                        />
                    </div>

                    <!-- Submit Button -->
                    <button
                        type="submit"
                        id="roll-btn"
                        class="w-full py-3 border-2 border-gray-500 text-gray-300 rounded-md <?= $user[
                            "logged_in"
                        ]
                            ? "hover:bg-gray-700 hover:text-white"
                            : "" ?>"
                        <?= !$user["logged_in"] ? "disabled" : "" ?>
                    >
                        <?= $user["logged_in"]
                            ? "Roll Dice"
                            : "Log in to roll!" ?>
                    </button>
                </form>

                <!-- Bet Result Display -->
                <?php if ($bet_result !== null || $bet_error !== null): ?>
                <div id="bet-result" class="bet-result <?= $bet_error
                    ? "error"
                    : ($bet_result["won"]
                        ? "win"
                        : "lose") ?>">
                    <?php if ($bet_error): ?>
                        <div class="result-title">Error!</div>
                        <div><?= htmlspecialchars($bet_error) ?></div>
                    <?php else: ?>
                        <div class="result-title">
                            <?= $bet_result["won"] ? "YOU WON!" : "YOU LOST!" ?>
                        </div>
                        <div class="result-details">
                            <div class="detail-item">
                                <span class="detail-label">Rolled</span>
                                <span class="detail-value"><?= number_format(
                                    $bet_result["result"] / 100,
                                    2
                                ) ?></span>
                            </div>
                            <div class="detail-item">
                                <span class="detail-label">Target</span>
                                <span class="detail-value">
                                    <?= ($_POST["is_under"] === "true"
                                        ? "< "
                                        : "> ") .
                                        number_format($_POST["threshold"], 2) ?>
                                </span>
                            </div>
                            <div class="detail-item">
                                <span class="detail-label">Bet Amount</span>
                                <span class="detail-value"><?= number_format(
                                    $_POST["bet_amount"],
                                    2
                                ) ?></span>
                            </div>
                            <div class="detail-item">
                                <span class="detail-label">Profit</span>
                                <span class="detail-value">
                                    <?= ($bet_result["won"] ? "+" : "-") .
                                        number_format(
                                            abs($bet_result["deltaCents"]) /
                                                100,
                                            2
                                        ) ?>
                                </span>
                            </div>
                        </div>
                        <?php if (isset($bet_result["serverSeed"])): ?>
                        <div style="margin-top: 15px; font-size: 0.8rem; opacity: 0.7;">
                            Server Seed: <?= htmlspecialchars(
                                $bet_result["serverSeed"]
                            ) ?>
                        </div>
                        <?php endif; ?>
                    <?php endif; ?>
                </div>
                <?php endif; ?>

                <!-- Recent Bets Table -->
                <?php if ($user["logged_in"] && !empty($recent_bets)): ?>
                <div class="recent-bets-table">
                    <h3 class="text-lg font-bold mb-4">Recent Bets</h3>
                    <table>
                        <thead>
                            <tr>
                                <th>Time</th>
                                <th>Bet</th>
                                <th>Target</th>
                                <th>Roll</th>
                                <th>Payout</th>
                            </tr>
                        </thead>
                        <tbody id="bets-tbody">
                            <?php foreach ($recent_bets as $bet): ?>
                            <tr>
                                <td><?= date("H:i:s", $bet["createdAt"]) ?></td>
                                <td><?= number_format(
                                    $bet["amountCents"] / 100,
                                    2
                                ) ?></td>
                                <td><?= ($bet["rollUnder"] ? "< " : "> ") .
                                    number_format(
                                        $bet["threshold"] / 100,
                                        2
                                    ) ?></td>
                                <td><?= number_format(
                                    $bet["result"] / 100,
                                    2
                                ) ?></td>
                                <td class="<?= $bet["won"] ? "win" : "lose" ?>">
                                    <?php if ($bet["won"]): ?>
                                        <?php
                                        $underAmount = $bet["rollUnder"]
                                            ? $bet["threshold"]
                                            : 10000 - $bet["threshold"];
                                        $payout =
                                            ($bet["amountCents"] *
                                                10000 *
                                                (100 - 1)) /
                                            ($underAmount * 100);
                                        echo "+" .
                                            number_format(
                                                ($payout -
                                                    $bet["amountCents"]) /
                                                    100,
                                                2
                                            );
                                        ?>
                                    <?php else: ?>
                                        -<?= number_format(
                                            $bet["amountCents"] / 100,
                                            2
                                        ) ?>
                                    <?php endif; ?>
                                </td>
                            </tr>
                            <?php endforeach; ?>
                        </tbody>
                    </table>
                </div>
                <?php endif; ?>
            </div>
        </main>

        <p id="max-bet" class="hidden"><?= $max_bet ?></p>
        <p id="server-seed-hash" class="hidden"><?= $user[
            "serverSeedHash"
        ] ?></p>

        <script type="text/javascript" src="index.js"></script>

        <?php if ($bet_result !== null && !$bet_error): ?>
        <script>
            // Update the balance display after successful bet
            document.getElementById('user-balance').textContent = <?= json_encode(
                number_format($user["balance"], 2)
            ) ?>;
        </script>
        <?php endif; ?>
    </body>
</html>
