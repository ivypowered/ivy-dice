const HOUSE_EDGE = 0.01; // 1%
const UNDER_MIN = 1.0;
const UNDER_MAX = 98.02;
const OVER_MIN = 1.97;
const OVER_MAX = 98.99;

function getId(id) {
    const element = document.getElementById(id);
    if (!element) {
        throw new Error("can't find " + id);
    }
    return element;
}

function round2(x) {
    return Math.round(x * 100) / 100;
}

function floor2(x) {
    return Math.floor(x * 100) / 100;
}

/** @type HTMLInputElement */
const betAmount = getId("bet-amount");
/** @type HTMLInputElement */
const betSlider = getId("bet-slider");
const betSliderBg = getId("bet-slider-bg");
const winChance = getId("win-chance");
const rollThreshold = getId("roll-amount");
const rollUo = getId("roll-uo");
const rollUoText = getId("roll-uo-text");
const payout = getId("payout");
/** @type HTMLInputElement */
const profit = getId("profit");
const maxBtn = getId("max-btn");
const balance =
    parseFloat(getId("user-balance").innerText.replace(",", "").trim()) || 0.0;
const maxBet =
    parseFloat(getId("max-bet").innerText.replace(",", "").trim()) || Infinity;
const isUnder = getId("is-under");
const threshold = getId("threshold");

const state = {
    betAmount: parseFloat(betAmount.value) || 0,
    isUnder: parseFloat(isUnder.value) || true,
    rollThreshold: parseFloat(threshold.value) || 49.5,
    invalid: false,
};
const computed = {
    winChance: () =>
        state.isUnder ? state.rollThreshold : 99.99 - state.rollThreshold,
    payout: () => (1 / (computed.winChance() / 100)) * (1 - HOUSE_EDGE),
    profit: () => floor2(state.betAmount * (computed.payout() - 1)),
};
const actions = {
    toggle: () => {
        if (state.invalid) {
            return;
        }
        state.isUnder = !state.isUnder;
        state.rollThreshold = round2(99.99 - state.rollThreshold);
    },
    setRollThreshold: (threshold) => {
        state.rollThreshold = threshold;
        state.invalid = false;
    },
    setWinChance: (winChance) => {
        if (state.isUnder) {
            if (winChance < UNDER_MIN) {
                state.invalid = true;
                return;
            }
            state.rollThreshold = winChance;
            state.invalid = false;
        } else {
            if (winChance < OVER_MIN) {
                state.invalid = true;
                return;
            }
            state.rollThreshold = round2(99.99 - winChance);
            state.invalid = false;
        }
    },
    setBetAmount: (betAmount) => {
        state.betAmount = betAmount;
    },
    recover: () => {
        if (!state.invalid) {
            return;
        }
        state.rollThreshold = state.isUnder ? 49.5 : 50.49;
        state.invalid = false;
    },
};
function render(exclude) {
    if (state.invalid) {
        // zero-width space
        rollThreshold.innerText = "\u200B";
        payout.value = "";
        profit.value = "";
        betSlider.value = betSlider.min;
        betSliderBg.style.width = 0;
        return;
    }
    if (exclude !== "betAmount") {
        if (!state.betAmount) {
            betAmount.value = "";
        } else {
            betAmount.value = state.betAmount;
        }
    }
    if (exclude !== "betSlider") {
        rollUoText.innerText = state.isUnder ? "UNDER" : "OVER";
        if (state.isUnder) {
            betSlider.min = UNDER_MIN;
            betSlider.max = UNDER_MAX;
        } else {
            betSlider.min = OVER_MIN;
            betSlider.max = OVER_MAX;
        }
        betSlider.value = state.rollThreshold;
        // linear conversion
        // of (1, 98.02) -> (0, 100)
        let percentage = ((state.rollThreshold - 1) / 97.02) * 100;
        // so that it is always hidden behind thumb
        if (percentage > 60) {
            percentage--;
        } else if (percentage > 3 && percentage < 6) {
            percentage++;
        } else if (percentage <= 3) {
            percentage += 2;
        }
        betSliderBg.style.width = percentage.toFixed(2) + "%";
    }
    if (exclude !== "rollThreshold") {
        rollThreshold.innerText = state.rollThreshold.toFixed(2);
    }
    if (exclude !== "winChance") {
        winChance.value = computed.winChance().toFixed(2);
    }
    if (exclude !== "payout") {
        payout.value = computed.payout().toFixed(2);
    }
    if (exclude !== "profit") {
        const p = computed.profit();
        if (!p) {
            profit.value = "";
        } else {
            profit.value = p;
        }
    }
    if (exclude !== "isUnder") {
        isUnder.value = String(state.isUnder);
    }
    if (exclude !== "threshold") {
        threshold.value = state.rollThreshold.toString();
    }
}

maxBtn.onclick = function () {
    actions.setBetAmount(Math.min(balance, maxBet));
    render("");
};
betAmount.oninput = function () {
    actions.setBetAmount(parseFloat(betAmount.value) || 0);
    render("betAmount");
};
rollUo.onclick = function () {
    actions.toggle();
    render("");
};
betSlider.oninput = function (e) {
    actions.setRollThreshold(parseFloat(e.target.value) || 0);
    render("");
};
winChance.oninput = function (e) {
    actions.setWinChance(parseFloat(e.target.value) || 0);
    render("winChance");
};
window.addEventListener("click", function (e) {
    // recovery from invalid state
    if (!state.invalid) {
        return;
    }
    if (winChance.contains(e.target) || payout.contains(e.target)) {
        return;
    }
    actions.recover();
    render("");
});

if (document.getElementById("account-dropdown")) {
    const accountDropdown = getId("account-dropdown");
    document.addEventListener("click", (e) => {
        if (!accountDropdown.contains(e.target)) {
            accountDropdown.removeAttribute("open");
        }
    });
}

const parentOrigin = new URLSearchParams(document.location.search).get(
    "parentOrigin",
);
const loginBtn = document.getElementById("login-btn");
if (parentOrigin && loginBtn) {
    parent.postMessage({ action: "subscribe" }, parentOrigin);
    window.addEventListener("message", function (event) {
        if (event.origin !== parentOrigin) return;
        const d = event.data;
        if (!d.user) {
            loginBtn.classList.remove("disabled");
            loginBtn.innerText = "Connect Wallet";
            loginBtn.onclick = function () {
                parent.postMessage({ action: "connect_wallet" }, parentOrigin);
            };
            return;
        }
        if (!d.signature || !d.message) {
            loginBtn.classList.remove("disabled");
            loginBtn.innerText = "Sign In";
            loginBtn.onclick = function () {
                parent.postMessage({ action: "sign_message" }, parentOrigin);
            };
            return;
        }
        document.cookie =
            "Message=" +
            encodeURIComponent(d.message) +
            "; max-age=86400; path=/; secure; samesite=strict";
        document.cookie =
            "Signature=" +
            encodeURIComponent(d.signature) +
            "; max-age=86400; path=/; secure; samesite=strict";
        window.location.reload();
    });
}

const logoutBtn = document.getElementById("logout-btn");
if (logoutBtn) {
    logoutBtn.onclick = function () {
        document.cookie =
            "Message=; path=/; secure; samesite=strict; expires=Thu, 01 Jan 1970 00:00:00 GMT";
        document.cookie =
            "Signature=; path=/; secure; samesite=strict; expires=Thu, 01 Jan 1970 00:00:00 GMT";
        if (parentOrigin) {
            parent.postMessage({ action: "logout" }, parentOrigin);
        }
        window.location.reload();
    };
}

// Get client seed
/** @type HTMLInputElement */
const clientSeed = getId("client-seed");
const csBytes = new Uint8Array(16);
crypto.getRandomValues(csBytes);
let csHex = "";
for (let i = 0; i < csBytes.length; i++) {
    const b = csBytes[i];
    const p = [b & 0xf, b >> 4];
    for (const v of p) {
        if (v >= 0 && v <= 9) {
            csHex += String.fromCharCode(
                48 + // '0'
                    v,
            );
        } else if (v >= 0xa && v <= 0xf) {
            csHex += String.fromCharCode(
                97 + // 'a'
                    (v - 0xa),
            );
        }
    }
}
clientSeed.value = csHex;

// Initial render
render("");
