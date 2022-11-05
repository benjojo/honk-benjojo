export function addguesscontrols(elem, word, wordlist, xid) {
	var host = elem.parentElement
	elem.innerHTML = "loading..."

	host.correctAnswer = word
	host.guesses = []
	host.xid = xid
	var xhr = new XMLHttpRequest()
        xhr.open("GET", "/bloat/wonkles?w=" + escape(wordlist))
        xhr.responseType = "json"
        xhr.onload = function() { 
		var wordlist = xhr.response.wordlist
		var validguesses = {}
		console.log("valid " + wordlist.length)
		for (var i = 0; i < wordlist.length; i++) {
			validguesses[wordlist[i]] = true
		}
		host.validGuesses = validguesses
		var div = document.createElement( 'div' );
		div.innerHTML = "<p><input> <button onclick='return makeaguess(this)'>guess</button>"
		host.append(div)
		elem.remove()
	}
        xhr.send()
}
export function makeaguess(btn) {
	var host = btn.parentElement.parentElement.parentElement
	var correct = host.correctAnswer
	var valid = host.validGuesses
	var inp = btn.previousElementSibling
	var g = inp.value.toLowerCase()
	var res = ""
	if (valid[g]) {
		var letters = {}
		var obfu = ""
		for (var i = 0; i < correct.length; i++) {
			var l = correct[i]
			letters[l] = (letters[l] | 0) + 1
		}
		for (var i = 0; i < g.length && i < correct.length; i++) {
			if (g[i] == correct[i]) {
				letters[g[i]] = letters[g[i]] - 1
			}
		}
		for (var i = 0; i < g.length; i++) {
			if (i < correct.length && g[i] == correct[i]) {
				res += g[i].toUpperCase()
				obfu += "&#129001;"
			} else if (letters[g[i]] > 0) {
				res += g[i]
				obfu += "&#129000;"
				letters[g[i]] = letters[g[i]] - 1
			} else {
				obfu += "&#11035;"
				res += "."
			}
		}

		var div = document.createElement( 'div' );
		div.innerHTML = "<p style='font-family: monospace'>" + res
		host.append(div)
		host.guesses.push(obfu)
	} else {
		var div = document.createElement( 'div' );
		div.innerHTML = "<p> invalid guess"
		host.append(div)
	}
	var div = document.createElement( 'div' );
	if (res == correct.toUpperCase()) {
		var mess = "<p>you are very smart!"
		mess += "<p>" + host.xid
		for (var i = 0; i < host.guesses.length; i++) {
			mess += "<p>" + host.guesses[i]
		}
		div.innerHTML = mess
		if (typeof(csrftoken) != "undefined")
			post("/zonkit", encode({"CSRF": csrftoken, "wherefore": "wonk", "guesses": host.guesses.join("<p>"), "what": host.xid}))
	} else {
		div.innerHTML = "<p><input> <button onclick='return makeaguess(this)'>guess</button>"
	}
	host.append(div)
	btn.parentElement.remove()
}
