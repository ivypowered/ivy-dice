<?php
const BACKEND_URL = "http://127.0.0.1:8000";

/**
 * Call the dice backend, throwing an exception if there was an error
 * either in the protocol or in the backend's response
 * @param array $data The JSON data to send to the backend
 * @return array The successful response from the backend
 */
function call_backend($data)
{
    $ch = curl_init();

    // Set options
    $options = [
        CURLOPT_URL => BACKEND_URL,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_CONNECTTIMEOUT => 5, // Connection timeout (seconds)
        CURLOPT_TIMEOUT => 15, // Total timeout (seconds)
        CURLOPT_CUSTOMREQUEST => "POST",
        CURLOPT_POSTFIELDS => json_encode($data),
    ];
    if (json_last_error() !== JSON_ERROR_NONE) {
        curl_close($ch);
        throw new Exception(json_last_error_msg());
    }
    curl_setopt_array($ch, $options);

    // Execute and get response
    $response = curl_exec($ch);
    $http_code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    $error = curl_error($ch);
    curl_close($ch);

    // Check for curl errors
    if ($error !== "") {
        throw new Exception($error);
    }

    // Check for empty response
    if (!$response || !is_string($response)) {
        throw new Exception("empty response from API");
    }

    // JSON decode response
    $decoded = json_decode($response, true);
    if (json_last_error() !== JSON_ERROR_NONE) {
        throw new Exception(json_last_error_msg());
    }

    // Check for `error`
    if (is_array($decoded) && isset($decoded["error"])) {
        throw new Exception($decoded["error"]);
    }

    // Return response
    return $decoded;
}

/**
 * SVG inline helper function
 * Creates inline SVGs with the provided class name
 *
 * @param string $icon Name of the SVG file (without extension)
 * @param string $class_name Classes to add to the SVG
 * @return string The inline SVG with applied classes
 */
function icon($icon, $class_name = "")
{
    $icon_path = __DIR__ . "/{$icon}.svg";

    if (!file_exists($icon_path)) {
        return "<!-- SVG {$icon} not found -->";
    }

    $svg = file_get_contents($icon_path);

    if (!empty($class_name)) {
        $svg = preg_replace(
            "/<svg([^>]*)>/",
            '<svg$1 class="' . htmlspecialchars($class_name) . '">',
            $svg,
            1,
        );
    }

    return $svg;
}

/**
 * Authenticate the user with the backend.
 * @return array The user object: { id: string; balance: number, logged_in: boolean, message: string, signature: string }
 */
function authenticate()
{
    $message =
        isset($_COOKIE["Message"]) && is_string($_COOKIE["Message"])
            ? $_COOKIE["Message"]
            : "";
    $signature =
        isset($_COOKIE["Signature"]) && is_string($_COOKIE["Signature"])
            ? $_COOKIE["Signature"]
            : "";

    $user_id = "";
    $balance = 0.0;
    $serverSeedHash = "";
    $is_logged_in = false;

    if ($message !== "" && $signature !== "") {
        $user = call_backend([
            "action" => "user_get",
            "message" => $message,
            "signature" => $signature,
        ]);
        $user_id = $user["id"];
        $balance = $user["balanceCents"] / 100.0;
        $serverSeedHash = $user["serverSeedHash"];
        $is_logged_in = true;
    }

    return [
        "id" => $user_id,
        "balance" => $balance,
        "logged_in" => $is_logged_in,
        "message" => $message,
        "signature" => $signature,
        "serverSeedHash" => $serverSeedHash,
    ];
}
