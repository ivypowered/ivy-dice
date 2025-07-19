<?php
require_once __DIR__ . "/util.php";

$user = authenticate();
if (!$user["logged_in"]) {
    header("Location: /");
    exit();
}

$error_message = "";
$success_message = "";
$withdraw_url = "";

// Handle form submissions
if ($_SERVER["REQUEST_METHOD"] === "POST") {
    if (isset($_POST["create_withdraw"])) {
        // Handle withdrawal creation
        $amount = floatval($_POST["amount"] ?? 0);
        if ($amount > 0) {
            if ($amount * 100 > $user["balance"] * 100) {
                $error_message = "Insufficient balance for this withdrawal.";
            } else {
                try {
                    $result = call_backend([
                        "action" => "withdraw",
                        "message" => $user["message"],
                        "signature" => $user["signature"],
                        "amountCents" => intval($amount * 100),
                    ]);
                    $success_message = "Withdrawal created successfully!";
                    $withdraw_url = $result["url"];
                    // Update local balance
                    $user["balance"] -= $amount;
                } catch (Exception $e) {
                    $error_message = $e->getMessage();
                }
            }
        } else {
            $error_message = "Please enter a valid amount";
        }
    }
}

// Fetch recent withdrawals
$recent_withdrawals = call_backend([
    "action" => "withdraw_list",
    "message" => $user["message"],
    "signature" => $user["signature"],
    "count" => 10,
    "skip" => 0,
]);
?>

<!doctype html>
<html lang="en">
    <head>
        <meta charset="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>Withdraw - Ivy Dice</title>
        <link rel="stylesheet" type="text/css" href="styles.css">
        <style>
            input[type="number"]::-webkit-inner-spin-button,
            input[type="number"]::-webkit-outer-spin-button {
                -webkit-appearance: none;
                margin: 0;
            }
            input[type="number"] {
                -moz-appearance: textfield;
                appearance: textfield;
            }
        </style>
    </head>
    <body class="bg-gray-900 text-white min-h-screen">
        <main class="container mx-auto p-4">
            <div class="max-w-2xl mx-auto">
                <!-- Header -->
                <div class="flex justify-between items-center mb-8">
                    <div>
                        <a href="/" class="text-gray-400 hover:text-gray-200 text-sm mb-2 inline-block">
                            ← Back to Home
                        </a>
                        <h1 class="text-2xl font-bold">Withdraw</h1>
                    </div>
                    <div class="text-sm text-gray-400">
                        Balance: <?= icon(
                            "dice-6",
                            "h-4 w-4 inline align-middle text-gray-400"
                        ) ?>
                        <span><?= number_format($user["balance"], 2) ?></span>
                    </div>
                </div>

                <!-- Messages -->
                <?php if ($error_message): ?>
                    <div class="bg-red-900/20 border border-red-600 text-red-400 p-3 rounded-md mb-4">
                        <?= htmlspecialchars($error_message) ?>
                    </div>
                <?php endif; ?>

                <?php if ($success_message): ?>
                    <div class="bg-green-900/20 border border-green-600 text-green-400 p-3 rounded-md mb-4">
                        <?= htmlspecialchars($success_message) ?>
                        <?php if ($withdraw_url): ?>
                            <br>
                            <a href="<?= htmlspecialchars(
                                $withdraw_url
                            ) ?>" target="_blank" class="text-blue-400 hover:text-blue-300 underline">
                                Click here to complete your withdrawal
                            </a>
                        <?php endif; ?>
                    </div>
                <?php endif; ?>

                <!-- Withdraw Form -->
                <div class="bg-gray-800 rounded-lg p-6 mb-8">
                    <h2 class="text-lg font-semibold mb-4">New Withdrawal</h2>
                    <form method="POST" class="space-y-4">
                        <div>
                            <label for="amount" class="block text-sm text-gray-400 mb-2">
                                Amount
                            </label>
                            <div class="relative">
                                <span class="absolute left-3 top-1/2 -translate-y-1/2">
                                    <?= icon(
                                        "dice-6",
                                        "h-4 w-4 text-gray-400"
                                    ) ?>
                                </span>
                                <input
                                    type="number"
                                    id="amount"
                                    name="amount"
                                    class="w-full pl-10 pr-3 py-2 bg-gray-700 border border-gray-600 rounded-md text-white placeholder-gray-500 focus:outline-none focus:border-gray-500"
                                    placeholder="0.00"
                                    min="0.01"
                                    max="<?= number_format(
                                        $user["balance"],
                                        2,
                                        ".",
                                        ""
                                    ) ?>"
                                    step="0.01"
                                    required
                                />
                            </div>
                        </div>
                        <button
                            type="submit"
                            name="create_withdraw"
                            value="1"
                            class="w-full py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
                        >
                            Create Withdrawal
                        </button>
                    </form>
                </div>

                <!-- Recent Withdrawals -->
                <div class="bg-gray-800 rounded-lg p-6">
                    <h2 class="text-lg font-semibold mb-4">Recent Withdrawals</h2>
                    <?php if (empty($recent_withdrawals)): ?>
                        <p class="text-gray-400 text-center py-8">No withdrawals yet</p>
                    <?php else: ?>
                        <div class="overflow-x-auto">
                            <table class="w-full">
                                <thead>
                                    <tr class="text-left text-gray-400 text-sm border-b border-gray-700">
                                        <th class="pb-2">ID</th>
                                        <th class="pb-2">Amount</th>
                                        <th class="pb-2">Date</th>
                                    </tr>
                                </thead>
                                <tbody class="text-sm">
                                    <?php foreach (
                                        $recent_withdrawals
                                        as $withdrawal
                                    ): ?>
                                        <tr class="border-b border-gray-700">
                                            <td class="py-3 pr-4">
                                                <a class="font-mono text-xs underline" href="<?= $withdrawal[
                                                    "url"
                                                ] ?>" target="_blank">
                                                    <?= htmlspecialchars(
                                                        substr(
                                                            $withdrawal["id"],
                                                            0,
                                                            8
                                                        )
                                                    ) ?>...
                                                </a>
                                            </td>
                                            <td class="py-3 pr-4">
                                                <?= icon(
                                                    "dice-6",
                                                    "h-3 w-3 inline align-middle text-gray-400 mr-1"
                                                ) ?>
                                                <?= number_format(
                                                    $withdrawal["amountCents"] /
                                                        100.0,
                                                    2
                                                ) ?>
                                            </td>
                                            <td class="py-3 pr-4">
                                                <?php
                                                $date = new DateTime();
                                                $date->setTimestamp(
                                                    $withdrawal["createdAt"]
                                                );
                                                echo $date->format("Y-m-d H:i");
                                                ?>
                                            </td>
                                        </tr>
                                    <?php endforeach; ?>
                                </tbody>
                            </table>
                        </div>
                    <?php endif; ?>
                </div>
            </div>
        </main>
    </body>
</html>
