<?php

$connections = [
    'workspace' => ['workspace-postgres', 'workspace', '/run/secrets/adminer/workspace-password'],
    'identity' => ['identity-postgres', 'identity', '/run/secrets/adminer/identity-password'],
];

$selection = $_GET['local'] ?? (empty($_GET) ? 'workspace' : null);
if ($_SERVER['REQUEST_METHOD'] === 'GET' && $selection !== null) {
    if (!is_string($selection) || !isset($connections[$selection])) {
        http_response_code(400);
        exit('Unknown local database.');
    }

    [$server, $username, $secret] = $connections[$selection];
    $password = file_get_contents($secret);
    if ($password === false || $password === '') {
        http_response_code(503);
        exit('Local database password is unavailable.');
    }

    \Adminer\set_password('pgsql', $server, $username, $password);
    header('Location: /?pgsql=' . $server . '&username=' . $username . '&db=' . $username);
    exit;
}

return new class extends \Adminer\Plugin {};
